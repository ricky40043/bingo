package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const testInk = "data:image/png;base64,aGVsbG8="

func config(n, target, winners int) Settings {
	return Settings{Size: n, Theme: "運動", Target: target, Winners: winners, JoinMode: "name"}
}
func TestLines(t *testing.T) {
	for _, n := range []int{3, 4} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			marks := map[int]Signature{}
			if count, _ := countLines(n, marks); count != 0 {
				t.Fatal("empty board has lines")
			}
			for i := 0; i < n; i++ {
				marks[i*n+i] = Signature{Name: "朋友"}
			}
			if count, cells := countLines(n, marks); count != 1 || len(cells) != n {
				t.Fatalf("diagonal: %d %v", count, cells)
			}
			marks = map[int]Signature{}
			for i := 0; i < n; i++ {
				marks[i*n+n-1-i] = Signature{Name: "朋友"}
			}
			if count, _ := countLines(n, marks); count != 1 {
				t.Fatal("opposite diagonal missing")
			}
			marks = map[int]Signature{}
			for i := 0; i < n; i++ {
				marks[i] = Signature{Name: "朋友"}
				marks[i*n] = Signature{Name: "朋友"}
			}
			if count, cells := countLines(n, marks); count != 2 || len(cells) != n*2-1 {
				t.Fatalf("row/column intersection: %d %v", count, cells)
			}
			for i := 0; i < n*n; i++ {
				marks[i] = Signature{Name: "朋友"}
			}
			if count, cells := countLines(n, marks); count != 2*n+2 || len(cells) != n*n {
				t.Fatalf("full board: %d %v", count, cells)
			}
		})
	}
}
func TestSettings(t *testing.T) {
	cases := []Settings{config(2, 1, 1), config(3, 9, 1), config(4, 0, 1), config(3, 1, 0), config(3, 1, 101), {Size: 3, Target: 1, Winners: 1, Theme: "不存在"}, {Size: 3, Target: 1, Winners: 1, Theme: "自訂", Items: []string{"a", "a", "b"}}, {Size: 3, Target: 1, Winners: 1, Theme: "運動", JoinMode: "wrong"}}
	for _, s := range cases {
		if validate(&s) == nil {
			t.Fatalf("accepted invalid settings: %+v", s)
		}
	}
	for _, n := range []int{3, 4} {
		r, err := newRoom("TEST", config(n, 2, 3))
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, q := range r.Board {
			if seen[q] {
				t.Fatal("duplicate question")
			}
			seen[q] = true
		}
		if len(r.Board) != n*n {
			t.Fatal("wrong board size")
		}
	}
}

func TestAutomaticNicknamesAreUnique(t *testing.T) {
	r, err := newRoom("TEST", Settings{Size: 3, Theme: "運動", Target: 1, Winners: 1, JoinMode: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := 0; i < 300; i++ {
		name, err := r.randomNickname()
		if err != nil {
			t.Fatal(err)
		}
		if seen[name] {
			t.Fatalf("duplicate nickname: %s", name)
		}
		seen[name] = true
		r.Players = append(r.Players, &Player{Name: name})
	}
	if len(nicknameAdjectives) != 20 || len(nicknameAnimals) != 30 {
		t.Fatalf("nickname pools changed: %d adjectives, %d animals", len(nicknameAdjectives), len(nicknameAnimals))
	}
}

func TestCallerModeAnswerCallAndClaim(t *testing.T) {
	r, err := newRoom("CALLER", Settings{
		Size: 4, Theme: "大專學校", Target: 1, Winners: 1,
		JoinMode: "auto", Mode: "caller", Groups: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Settings.Size != 3 || len(r.Board) != 9 {
		t.Fatalf("caller board is not 3x3: size=%d board=%d", r.Settings.Size, len(r.Board))
	}
	p := &Player{Name: "勇敢的水獺", Group: groupLabel(0), Answers: map[int]string{}, Marks: map[int]Signature{}}
	for i, answer := range []string{"臺灣大學", "政治大學", "淡江大學", "輔仁大學", "東吳大學", "世新大學", "文化大學", "北科大", "臺科大"} {
		if err := r.setAnswer(p, i, answer); err != nil {
			t.Fatal(err)
		}
	}
	if !p.Ready {
		t.Fatal("player should be ready after nine answers")
	}
	if err := r.setAnswer(p, 8, " 臺灣 大學 "); err == nil {
		t.Fatal("normalized duplicate answer was accepted")
	}
	r.Status = "playing"
	if err := r.claim(p, 0); err == nil {
		t.Fatal("uncalled answer was claimed")
	}
	for _, answer := range []string{"台灣大學", "政治大學", "淡江大學"} {
		if err := r.callAnswer(answer); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.callAnswer("政治 大學"); err == nil {
		t.Fatal("duplicate called answer was accepted")
	}
	for _, index := range []int{0, 1, 2} {
		if err := r.claim(p, index); err != nil {
			t.Fatal(err)
		}
	}
	if p.Rank != 1 || p.Lines != 1 || r.Status != "finished" {
		t.Fatalf("caller bingo failed: rank=%d lines=%d status=%s", p.Rank, p.Lines, r.Status)
	}
}
func TestSignRules(t *testing.T) {
	r, _ := newRoom("TEST", config(3, 1, 1))
	p := &Player{Marks: map[int]Signature{}}
	if r.sign(p, 0, Signature{Ink: testInk}) == nil {
		t.Fatal("signed before start")
	}
	r.Status = "playing"
	for _, i := range []int{-1, 9} {
		if r.sign(p, i, Signature{Ink: testInk}) == nil {
			t.Fatal("invalid cell accepted")
		}
	}
	if r.sign(p, 0, Signature{}) == nil {
		t.Fatal("blank signature")
	}
	if r.sign(p, 0, Signature{Ink: "javascript:test"}) == nil {
		t.Fatal("invalid image")
	}
	for i := 0; i < 3; i++ {
		if err := r.sign(p, i, Signature{Ink: testInk}); err != nil {
			t.Fatal(err)
		}
	}
	if p.Rank != 1 || r.Status != "finished" || len(r.Winners) != 1 {
		t.Fatal("did not finish")
	}
	if r.sign(p, 3, Signature{Ink: testInk}) == nil {
		t.Fatal("signed after end")
	}
	r.Status = "playing"
	p.Rank = 0
	if r.sign(p, 0, Signature{Ink: testInk}) == nil {
		t.Fatal("duplicate accepted")
	}
}

type wsPeer struct {
	t           *testing.T
	conn        *websocket.Conn
	token, room string
}

func dialPeer(t *testing.T, url string) *wsPeer {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &wsPeer{t: t, conn: conn}
}
func (p *wsPeer) send(v any) {
	p.t.Helper()
	if err := p.conn.WriteJSON(v); err != nil {
		p.t.Fatal(err)
	}
}
func (p *wsPeer) receive(kind string) map[string]json.RawMessage {
	p.t.Helper()
	p.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var v map[string]json.RawMessage
		if err := p.conn.ReadJSON(&v); err != nil {
			p.t.Fatal(err)
		}
		var k string
		json.Unmarshal(v["type"], &k)
		if k == kind {
			return v
		}
	}
}
func (p *wsPeer) session() {
	v := p.receive("session")
	json.Unmarshal(v["token"], &p.token)
	json.Unmarshal(v["room"], &p.room)
}
func testServer(t *testing.T) (*Server, string) {
	s := NewServer()
	h := httptest.NewServer(http.HandlerFunc(s.serveWS))
	t.Cleanup(h.Close)
	return s, h.URL
}
func TestWebSocketFlowAndResume(t *testing.T) {
	_, url := testServer(t)
	host := dialPeer(t, url)
	host.send(Message{Type: "create", Settings: config(4, 2, 1)})
	host.session()
	hostState := host.receive("state")
	var room Room
	json.Unmarshal(hostState["room"], &room)
	player := dialPeer(t, url)
	player.send(Message{Type: "join", Room: host.room, Name: "小安"})
	player.session()
	v := player.receive("state")
	var pr Room
	json.Unmarshal(v["room"], &pr)
	if strings.Join(pr.Board, "|") != strings.Join(room.Board, "|") {
		t.Fatal("boards differ")
	}
	player.send(Message{Type: "start"})
	player.receive("error")
	player.send(Message{Type: "sign", Index: 0, Signature: Signature{Ink: testInk}})
	player.receive("error")
	host.send(Message{Type: "start"})
	player.receive("state")
	for _, i := range []int{0, 1, 2, 3, 4, 8, 12} {
		player.send(Message{Type: "sign", Index: i, Signature: Signature{Ink: testInk}})
		player.receive("state")
	}
	v = host.receive("state") // Host may have older queued snapshots; resume must always get latest.
	resumed := dialPeer(t, url)
	resumed.send(Message{Type: "resume", Room: host.room, Token: player.token})
	resumed.session()
	v = resumed.receive("state")
	json.Unmarshal(v["room"], &pr)
	if pr.Status != "finished" || len(pr.Winners) != 1 || pr.Winners[0].Lines != 2 {
		t.Fatalf("incorrect results: %+v", pr)
	}
	var me struct {
		Marks map[int]Signature `json:"marks"`
	}
	json.Unmarshal(v["me"], &me)
	if len(me.Marks) != 7 {
		t.Fatalf("lost marks: %v", me)
	}
	bad := dialPeer(t, url)
	bad.send(Message{Type: "resume", Room: host.room, Token: "bad"})
	bad.receive("error")
	resumed.send(Message{Type: "reset"})
	resumed.receive("error")
	host2 := dialPeer(t, url)
	host2.send(Message{Type: "resume", Room: host.room, Token: host.token})
	host2.session()
	hostState = host2.receive("state")
	var winnerBoards map[string]map[int]Signature
	json.Unmarshal(hostState["winnerBoards"], &winnerBoards)
	if len(winnerBoards[pr.Winners[0].ID]) != 7 {
		t.Fatalf("host cannot review winner signatures: %v", winnerBoards)
	}
	host2.send(Message{Type: "reset"})
	v = host2.receive("state")
	json.Unmarshal(v["room"], &pr)
	if pr.Status != "lobby" || pr.Round != 2 || len(pr.Winners) != 0 || pr.Players[0].Count != 0 {
		t.Fatalf("bad reset: %+v", pr)
	}
}

func TestSocialPlayersCanJoinAfterStart(t *testing.T) {
	_, url := testServer(t)
	host := dialPeer(t, url)
	host.send(Message{Type: "create", Settings: config(3, 2, 1)})
	host.session()
	host.receive("state")

	first := dialPeer(t, url)
	first.send(Message{Type: "join", Room: host.room, Name: "先加入"})
	first.session()
	first.receive("state")
	host.send(Message{Type: "start"})
	for {
		state := first.receive("state")
		var room Room
		json.Unmarshal(state["room"], &room)
		if room.Status == "playing" {
			break
		}
	}

	late := dialPeer(t, url)
	late.send(Message{Type: "join", Room: host.room, Name: "後加入"})
	late.session()
	state := late.receive("state")
	var room Room
	json.Unmarshal(state["room"], &room)
	if room.Status != "playing" || len(room.Players) != 2 || room.Players[1].Name != "後加入" {
		t.Fatalf("late social join failed: %+v", room)
	}
}

func TestWebSocketAutomaticJoinIgnoresSubmittedName(t *testing.T) {
	_, url := testServer(t)
	host := dialPeer(t, url)
	settings := config(3, 1, 1)
	settings.JoinMode = "auto"
	host.send(Message{Type: "create", Settings: settings})
	host.session()
	host.receive("state")

	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		player := dialPeer(t, url)
		player.send(Message{Type: "join", Room: host.room, Name: "偽造的名字"})
		player.session()
		state := player.receive("state")
		var room Room
		json.Unmarshal(state["room"], &room)
		name := room.Players[len(room.Players)-1].Name
		if name == "偽造的名字" {
			t.Fatal("automatic room accepted the submitted name")
		}
		if seen[name] {
			t.Fatalf("duplicate automatic nickname: %s", name)
		}
		seen[name] = true
	}
}

func TestCallerWebSocketFlow(t *testing.T) {
	_, url := testServer(t)
	host := dialPeer(t, url)
	settings := Settings{Size: 3, Theme: "大專學校", Target: 1, Winners: 1, JoinMode: "auto", Mode: "caller", Groups: 2}
	host.send(Message{Type: "create", Settings: settings})
	host.session()
	host.receive("state")

	player := dialPeer(t, url)
	player.send(Message{Type: "join", Room: host.room})
	player.receive("error")
	player.send(Message{Type: "join", Room: host.room, Group: "A組"})
	player.session()
	state := player.receive("state")
	var room Room
	json.Unmarshal(state["room"], &room)
	if room.Players[0].Group != "A組" || room.Players[0].Ready {
		t.Fatalf("unexpected initial caller player: %+v", room.Players[0])
	}
	second := dialPeer(t, url)
	second.send(Message{Type: "join", Room: host.room, Group: "A組"})
	second.session()
	secondState := second.receive("state")
	player.receive("state")
	json.Unmarshal(secondState["room"], &room)
	if len(room.Players) != 2 || room.Players[1].Group != "A組" {
		t.Fatalf("same-group join should be unlimited: %+v", room.Players)
	}
	second.send(Message{Type: "leave"})
	second.receive("left")
	player.receive("state")

	player.send(Message{Type: "answer", Index: 0, Value: "臺灣大學"})
	player.receive("state")
	host.send(Message{Type: "start"})
	host.receive("error")

	answers := []string{"臺灣大學", "政治大學", "淡江大學", "輔仁大學", "東吳大學", "世新大學", "文化大學", "北科大", "臺科大"}
	for index := 1; index < len(answers); index++ {
		player.send(Message{Type: "answer", Index: index, Value: answers[index]})
		state = player.receive("state")
	}
	var me struct {
		Answers map[int]string `json:"answers"`
	}
	json.Unmarshal(state["me"], &me)
	if len(me.Answers) != 9 {
		t.Fatalf("private answers missing: %v", me.Answers)
	}
	json.Unmarshal(state["room"], &room)
	if !room.Players[0].Ready {
		t.Fatal("public readiness did not update")
	}

	host.send(Message{Type: "start"})
	for {
		state = player.receive("state")
		json.Unmarshal(state["room"], &room)
		if room.Status == "playing" {
			break
		}
	}
	for index, answer := range answers[:3] {
		host.send(Message{Type: "call", Value: answer})
		player.receive("state")
		player.send(Message{Type: "claim", Index: index})
		state = player.receive("state")
	}
	json.Unmarshal(state["room"], &room)
	if room.Status != "finished" || len(room.Winners) != 1 || room.Winners[0].Group != "A組" {
		t.Fatalf("caller websocket game did not finish: %+v", room)
	}
}

func TestConcurrentWinnersAndDuplicateNames(t *testing.T) {
	s, url := testServer(t)
	host := dialPeer(t, url)
	host.send(Message{Type: "create", Settings: config(3, 1, 3)})
	host.session()
	host.receive("state")
	players := []*wsPeer{}
	for i := 0; i < 5; i++ {
		p := dialPeer(t, url)
		p.send(Message{Type: "join", Room: host.room, Name: "同名"})
		p.session()
		p.receive("state")
		players = append(players, p)
	}
	host.send(Message{Type: "start"})
	// Wait for authoritative playing state before preparing all boards.
	for {
		v := host.receive("state")
		var r Room
		json.Unmarshal(v["room"], &r)
		if r.Status == "playing" {
			break
		}
	}
	for _, p := range players {
		p.send(Message{Type: "sign", Index: 0, Signature: Signature{Ink: testInk}})
		p.send(Message{Type: "sign", Index: 1, Signature: Signature{Ink: testInk}})
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		count := 0
		for _, p := range s.rooms[host.room].Players {
			count += p.Count
		}
		s.mu.Unlock()
		if count == 10 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sign preparation timed out")
		}
		time.Sleep(time.Millisecond)
	}
	var wg sync.WaitGroup
	for _, p := range players {
		wg.Add(1)
		go func(p *wsPeer) {
			defer wg.Done()
			p.send(Message{Type: "sign", Index: 2, Signature: Signature{Ink: testInk}})
		}(p)
	}
	wg.Wait()
	deadline = time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		r := s.rooms[host.room]
		done := r.Status == "finished"
		if done {
			if len(r.Winners) != 3 {
				t.Fatal("too many winners")
			}
			count := 0
			for i, p := range r.Winners {
				if p.Rank != i+1 {
					t.Fatal("rank gap")
				}
			}
			for _, p := range r.Players {
				count += p.Count
			}
			if count != 13 {
				t.Fatalf("late signature accepted: %d", count)
			}
		}
		s.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("concurrent finish timed out")
		}
		time.Sleep(time.Millisecond)
	}
}
