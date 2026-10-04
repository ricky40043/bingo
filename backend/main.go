package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Server struct {
	mu      sync.Mutex
	rooms   map[string]*Room
	clients map[*Client]bool
}
type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	room   *Room
	player *Player
	host   bool
}
type Message struct {
	Type      string    `json:"type"`
	Room      string    `json:"room"`
	Token     string    `json:"token"`
	Name      string    `json:"name"`
	Group     string    `json:"group"`
	Settings  Settings  `json:"settings"`
	Index     int       `json:"index"`
	Value     string    `json:"value"`
	ID        string    `json:"id"`
	Decision  string    `json:"decision"`
	Called    string    `json:"called"`
	Signature Signature `json:"signature"`
}

func NewServer() *Server { return &Server{rooms: map[string]*Room{}, clients: map[*Client]bool{}} }
func (c *Client) emit(v any) {
	b, _ := json.Marshal(v)
	select {
	case c.send <- b:
	default:
		_ = c.conn.Close()
	}
}
func (s *Server) broadcast(r *Room) {
	r.HostOnline = false
	for _, p := range r.Players {
		p.Online = false
	}
	for c := range s.clients {
		if c.room == r {
			if c.host {
				r.HostOnline = true
			} else if c.player != nil {
				c.player.Online = true
			}
		}
	}
	for c := range s.clients {
		if c.room == r {
			var me any
			var winnerBoards map[string]map[int]Signature
			var playerAnswers map[string]map[int]string
			if c.player != nil {
				me = map[string]any{"id": c.player.ID, "marks": c.player.Marks, "answers": c.player.Answers}
			}
			if c.host {
				winnerBoards = make(map[string]map[int]Signature, len(r.Winners))
				for _, winner := range r.Winners {
					winnerBoards[winner.ID] = winner.Marks
				}
				if r.Settings.Mode == "turns" {
					playerAnswers = make(map[string]map[int]string, len(r.Players))
					for _, player := range r.Players {
						playerAnswers[player.ID] = player.Answers
					}
				}
			}
			c.emit(map[string]any{
				"type": "state", "room": r, "me": me, "host": c.host,
				"winnerBoards": winnerBoards, "playerAnswers": playerAnswers,
			})
		}
	}
}
func (s *Server) handle(c *Client, m Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fail := func(message string) { c.emit(map[string]any{"type": "error", "message": message}) }
	if m.Type == "create" || m.Type == "join" || m.Type == "resume" {
		if c.room != nil {
			fail("已經加入房間")
			return
		}
		var r *Room
		if m.Type == "create" {
			if len(s.rooms) >= 500 {
				fail("目前房間已滿，請稍後再試")
				return
			}
			id := strings.ToUpper(token()[:6])
			for s.rooms[id] != nil {
				id = strings.ToUpper(token()[:6])
			}
			var err error
			r, err = newRoom(id, m.Settings)
			if err != nil {
				fail(err.Error())
				return
			}
			s.rooms[id] = r
			c.host = true
		} else {
			r = s.rooms[strings.ToUpper(strings.TrimSpace(m.Room))]
			if r == nil {
				fail("找不到房間，可能已過期或伺服器已重啟")
				return
			}
			if m.Type == "resume" {
				if m.Token == r.HostToken {
					c.host = true
				} else {
					for _, p := range r.Players {
						if p.Token == m.Token {
							c.player = p
							break
						}
					}
					if c.player == nil {
						fail("無法恢復身分，請重新加入")
						return
					}
				}
			} else {
				if r.Status == "finished" || ((r.Settings.Mode == "caller" || r.Settings.Mode == "turns") && r.Status != "lobby") {
					fail("遊戲已開始，請等待下一場")
					return
				}
				if len(r.Players) >= 300 {
					fail("房間已滿（300 人）")
					return
				}
				var name string
				var err error
				if r.Settings.JoinMode == "auto" {
					name, err = r.randomNickname()
				} else {
					name, err = cleanName(m.Name)
				}
				if err != nil {
					fail(err.Error())
					return
				}
				group := ""
				if r.Settings.Mode == "caller" || r.Settings.Mode == "turns" {
					for groupIndex := 0; groupIndex < r.Settings.Groups; groupIndex++ {
						if m.Group == groupLabel(groupIndex) {
							group = m.Group
							break
						}
					}
					if group == "" {
						fail("請先選擇組別")
						return
					}
				}
				c.player = &Player{ID: token()[:12], Name: name, Token: token(), Marks: map[int]Signature{}, Answers: map[int]string{}, Cells: []int{}, Group: group}
				r.Players = append(r.Players, c.player)
			}
		}
		c.room = r
		r.Updated = time.Now()
		credential := r.HostToken
		if c.player != nil {
			credential = c.player.Token
		}
		c.emit(map[string]any{"type": "session", "room": r.ID, "token": credential, "host": c.host})
		s.broadcast(r)
		return
	}
	r := c.room
	if r == nil {
		fail("請先加入房間")
		return
	}
	r.Updated = time.Now()
	switch m.Type {
	case "leave":
		if c.host || c.player == nil {
			fail("只有玩家可以退出房間")
			return
		}
		if r.Status != "lobby" {
			fail("遊戲開始後不能重新選組")
			return
		}
		for index, player := range r.Players {
			if player == c.player {
				r.Players = append(r.Players[:index], r.Players[index+1:]...)
				break
			}
		}
		c.room = nil
		c.player = nil
		c.emit(map[string]any{"type": "left"})
		s.broadcast(r)
		return
	case "start":
		if !c.host {
			fail("只有主持人可以開始")
			return
		}
		if r.Status != "lobby" {
			fail("遊戲已經開始")
			return
		}
		if len(r.Players) < r.Settings.Winners {
			fail(fmt.Sprintf("至少需要 %d 位玩家才能開始", r.Settings.Winners))
			return
		}
		if r.Settings.Mode == "caller" || r.Settings.Mode == "turns" {
			for _, player := range r.Players {
				if !player.Ready {
					fail(fmt.Sprintf("請等所有玩家填滿 %d 格答案後再開始", r.Settings.Size*r.Settings.Size))
					return
				}
			}
		}
		r.Status = "playing"
		r.StartedAt = time.Now().UnixMilli()
		if r.Settings.Mode == "turns" {
			r.initializeTurn()
		}
	case "sign":
		if c.player == nil {
			fail("主持人不能代替玩家簽名")
			return
		}
		if err := r.sign(c.player, m.Index, m.Signature); err != nil {
			fail(err.Error())
			return
		}
	case "answer":
		if c.player == nil {
			fail("只有玩家可以填寫答案")
			return
		}
		if err := r.setAnswer(c.player, m.Index, m.Value); err != nil {
			fail(err.Error())
			return
		}
	case "call":
		if !c.host {
			fail("只有主持人可以公布答案")
			return
		}
		if err := r.callAnswer(m.Value); err != nil {
			fail(err.Error())
			return
		}
	case "claim":
		if c.player == nil {
			fail("只有玩家可以圈選答案")
			return
		}
		if err := r.claim(c.player, m.Index); err != nil {
			fail(err.Error())
			return
		}
	case "propose":
		if c.player == nil {
			fail("只有玩家可以代表組別提出答案")
			return
		}
		if err := r.propose(c.player, m.Index); err != nil {
			fail(err.Error())
			return
		}
	case "proposal_decision":
		if !c.host {
			fail("只有主持人可以審核答案")
			return
		}
		if err := r.reviewProposal(m.ID, m.Decision); err != nil {
			fail(err.Error())
			return
		}
	case "skip_turn":
		if !c.host {
			fail("只有主持人可以跳過組別")
			return
		}
		if err := r.skipTurn(); err != nil {
			fail(err.Error())
			return
		}
	case "appeal":
		if c.player == nil {
			fail("只有玩家可以提出同答案申請")
			return
		}
		if err := r.requestMatch(c.player, m.Index, m.Called); err != nil {
			fail(err.Error())
			return
		}
	case "appeal_decision":
		if !c.host {
			fail("只有主持人可以審核同答案申請")
			return
		}
		if err := r.reviewAppeal(m.ID, m.Decision); err != nil {
			fail(err.Error())
			return
		}
	case "reset":
		if !c.host {
			fail("只有主持人可以重開")
			return
		}
		if r.Status != "finished" {
			fail("遊戲結束後才能重開")
			return
		}
		r.Status = "lobby"
		r.Round++
		r.StartedAt = 0
		r.Winners = []*Player{}
		r.Called = []string{}
		r.TurnIndex = 0
		r.Pending = nil
		r.Appeals = []*MatchAppeal{}
		r.ApprovedMatches = map[string]string{}
		r.Board = draw(r.Settings)
		for _, p := range r.Players {
			p.Marks = map[int]Signature{}
			p.Lines = 0
			p.Cells = []int{}
			p.Count = 0
			p.Rank = 0
			p.FinishedAt = 0
			p.Answers = map[int]string{}
			p.Ready = false
		}
	case "end":
		if !c.host {
			fail("只有主持人可以結束")
			return
		}
		if r.Status != "playing" {
			fail("遊戲尚未開始")
			return
		}
		r.Status = "finished"
	default:
		fail("不支援的操作")
		return
	}
	s.broadcast(r)
}
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	up := websocket.Upgrader{CheckOrigin: func(req *http.Request) bool {
		origin := req.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && (u.Host == req.Host || origin == strings.TrimRight(os.Getenv("FRONTEND_URL"), "/"))
	}}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &Client{conn: conn, send: make(chan []byte, 32)}
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		defer conn.Close()
		for {
			select {
			case msg, ok := <-c.send:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if conn.WriteMessage(websocket.TextMessage, msg) != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if conn.WriteMessage(websocket.PingMessage, nil) != nil {
					return
				}
			}
		}
	}()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		close(c.send)
		if c.room != nil {
			s.broadcast(c.room)
		}
		s.mu.Unlock()
		conn.Close()
	}()
	conn.SetReadLimit(140000)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	for {
		var m Message
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		s.handle(c, m)
	}
}
func main() {
	s := NewServer()
	go func() {
		for range time.Tick(10 * time.Minute) {
			s.mu.Lock()
			for id, r := range s.rooms {
				active := false
				for c := range s.clients {
					if c.room == r {
						active = true
						break
					}
				}
				if !active && time.Since(r.Updated) > 2*time.Hour {
					delete(s.rooms, id)
				}
			}
			s.mu.Unlock()
		}
	}()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	router.GET("/api/themes", func(c *gin.Context) { c.JSON(200, themes) })
	router.GET("/api/caller-themes", func(c *gin.Context) { c.JSON(200, callerThemes) })
	router.GET("/api/config", func(c *gin.Context) { c.JSON(200, gin.H{"frontendUrl": os.Getenv("FRONTEND_URL")}) })
	router.GET("/api/rooms/:id", func(c *gin.Context) {
		s.mu.Lock()
		defer s.mu.Unlock()
		room := s.rooms[strings.ToUpper(strings.TrimSpace(c.Param("id")))]
		if room == nil || room.Status != "lobby" {
			c.JSON(http.StatusNotFound, gin.H{"message": "找不到可加入的房間"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"mode": room.Settings.Mode, "joinMode": room.Settings.JoinMode,
			"groups": room.Settings.Groups, "theme": room.Settings.Theme,
		})
	})
	router.GET("/ws", func(c *gin.Context) { s.serveWS(c.Writer, c.Request) })
	static := os.Getenv("STATIC_DIR")
	if static == "" {
		static = "../frontend/dist"
	}
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Status(404)
			return
		}
		path := filepath.Join(static, filepath.Clean("/"+c.Request.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			c.File(path)
			return
		}
		c.File(filepath.Join(static, "index.html"))
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	log.Printf("Bingo listening on :%s", port)
	log.Fatal(router.Run(":" + port))
}
