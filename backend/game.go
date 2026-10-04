package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Settings struct {
	Size     int      `json:"size"`
	Theme    string   `json:"theme"`
	Target   int      `json:"target"`
	Winners  int      `json:"winners"`
	Items    []string `json:"items"`
	JoinMode string   `json:"joinMode"`
	Mode     string   `json:"mode"`
	Groups   int      `json:"groups"`
}
type Signature struct {
	Name string `json:"name"`
	Ink  string `json:"ink,omitempty"`
}
type Player struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Token      string            `json:"-"`
	Marks      map[int]Signature `json:"-"`
	Lines      int               `json:"lines"`
	Cells      []int             `json:"cells"`
	Count      int               `json:"count"`
	Rank       int               `json:"rank"`
	FinishedAt int64             `json:"finishedAt,omitempty"`
	Online     bool              `json:"online"`
	Group      string            `json:"group,omitempty"`
	Answers    map[int]string    `json:"-"`
	Ready      bool              `json:"ready"`
}
type TurnProposal struct {
	ID         string `json:"id"`
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
	Group      string `json:"group"`
	Answer     string `json:"answer"`
}
type MatchAppeal struct {
	ID         string `json:"id"`
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
	Group      string `json:"group"`
	Index      int    `json:"index"`
	Answer     string `json:"answer"`
	Called     string `json:"called"`
}
type Room struct {
	ID              string            `json:"id"`
	Settings        Settings          `json:"settings"`
	Board           []string          `json:"board"`
	Status          string            `json:"status"`
	Players         []*Player         `json:"players"`
	Winners         []*Player         `json:"winners"`
	StartedAt       int64             `json:"startedAt"`
	Round           int               `json:"round"`
	HostOnline      bool              `json:"hostOnline"`
	HostToken       string            `json:"-"`
	Updated         time.Time         `json:"-"`
	Called          []string          `json:"called"`
	TurnIndex       int               `json:"turnIndex"`
	Pending         *TurnProposal     `json:"pending,omitempty"`
	Appeals         []*MatchAppeal    `json:"appeals"`
	ApprovedMatches map[string]string `json:"approvedMatches"`
}

var nicknameAdjectives = []string{
	"開心的", "勇敢的", "溫柔的", "好奇的", "活潑的",
	"冷靜的", "聰明的", "可愛的", "害羞的", "熱情的",
	"悠閒的", "認真的", "幸運的", "淘氣的", "善良的",
	"生氣的", "憂鬱的", "閃亮的", "神秘的", "愛笑的",
}

var nicknameAnimals = []string{
	"獅子", "羚羊", "水獺", "企鵝", "長頸鹿", "大象",
	"狐狸", "浣熊", "兔子", "無尾熊", "熊貓", "海豚",
	"海豹", "河馬", "斑馬", "袋鼠", "刺蝟", "松鼠",
	"貓頭鷹", "老虎", "花豹", "狼", "梅花鹿", "羊駝",
	"樹懶", "北極熊", "紅鶴", "鸚鵡", "烏龜", "鯨魚",
}

func token() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func cleanName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 80 {
		return "", errors.New("請輸入 1–80 個字的姓名（可重名）")
	}
	return s, nil
}
func validate(s *Settings) error {
	if s.Mode == "" {
		s.Mode = "social"
	}
	if s.Mode != "social" && s.Mode != "caller" && s.Mode != "turns" {
		return errors.New("遊戲模式設定錯誤")
	}
	if s.JoinMode == "" {
		s.JoinMode = "name"
	}
	if s.JoinMode != "name" && s.JoinMode != "auto" {
		return errors.New("加入方式設定錯誤")
	}
	if s.Mode == "caller" || s.Mode == "turns" {
		if s.Mode == "caller" {
			s.Size = 3
		} else {
			s.Size = 4
		}
		if s.Groups < 2 || s.Groups > 12 {
			return errors.New("分組數需介於 2–12 組")
		}
	}
	if s.Size != 3 && s.Size != 4 {
		return errors.New("請選擇九宮格或十六宮格")
	}
	if s.Target < 1 || s.Target > 2*s.Size+2 {
		return errors.New("目標線數超出範圍")
	}
	if s.Winners < 1 || s.Winners > 100 {
		return errors.New("得獎名額需介於 1–100 人")
	}
	s.Theme = strings.TrimSpace(s.Theme)
	if s.Theme == "" || utf8.RuneCountInString(s.Theme) > 80 {
		return errors.New("請填寫主題（最多 80 字）")
	}
	if s.Mode == "caller" || s.Mode == "turns" {
		if _, exists := callerThemes[s.Theme]; !exists {
			return errors.New("請選擇有效的猜答案題目類別")
		}
		s.Items = nil
		return nil
	}
	if len(s.Items) == 0 {
		s.Items = append([]string(nil), themes[s.Theme]...)
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, item := range s.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if utf8.RuneCountInString(item) > 100 {
			return errors.New("每個題目最多 100 字")
		}
		if !seen[item] {
			unique = append(unique, item)
			seen[item] = true
		}
	}
	if len(unique) < s.Size*s.Size || len(unique) > 200 {
		return errors.New("請提供足夠的不重複題目（九宮格至少 9 題、十六宮格至少 16 題，最多 200 題）")
	}
	s.Items = unique
	return nil
}

func randomIndex(length int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

func (r *Room) randomNickname() (string, error) {
	used := make(map[string]bool, len(r.Players))
	for _, player := range r.Players {
		used[player.Name] = true
	}

	total := len(nicknameAdjectives) * len(nicknameAnimals)
	if len(used) >= total {
		return "", errors.New("自動暱稱已用完，請建立新房間")
	}

	start := randomIndex(total)
	for offset := 0; offset < total; offset++ {
		combination := (start + offset) % total
		name := nicknameAdjectives[combination/len(nicknameAnimals)] + nicknameAnimals[combination%len(nicknameAnimals)]
		if !used[name] {
			return name, nil
		}
	}
	return "", errors.New("無法產生自動暱稱")
}
func draw(s Settings) []string {
	if s.Mode == "caller" || s.Mode == "turns" {
		return make([]string, s.Size*s.Size)
	}
	a := append([]string(nil), s.Items...)
	for i := len(a) - 1; i > 0; i-- {
		v, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(v.Int64())
		a[i], a[j] = a[j], a[i]
	}
	return a[:s.Size*s.Size]
}
func newRoom(id string, s Settings) (*Room, error) {
	if err := validate(&s); err != nil {
		return nil, err
	}
	return &Room{
		ID: id, Settings: s, Board: draw(s), Status: "lobby", Players: []*Player{},
		Winners: []*Player{}, HostToken: token(), Updated: time.Now(), Round: 1,
		Called: []string{}, Appeals: []*MatchAppeal{}, ApprovedMatches: map[string]string{},
	}, nil
}

func groupLabel(index int) string {
	return string(rune('A'+index)) + "組"
}

func normalizeAnswer(value string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
	return strings.ReplaceAll(normalized, "臺", "台")
}

func (r *Room) setAnswer(p *Player, index int, answer string) error {
	if (r.Settings.Mode != "caller" && r.Settings.Mode != "turns") || r.Status != "lobby" {
		return errors.New("現在不能修改答案")
	}
	boardCount := r.Settings.Size * r.Settings.Size
	if index < 0 || index >= boardCount {
		return errors.New("無效的格子")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" || utf8.RuneCountInString(answer) > 40 {
		return errors.New("答案需為 1–40 個字")
	}
	for answerIndex, existing := range p.Answers {
		if answerIndex != index && normalizeAnswer(existing) == normalizeAnswer(answer) {
			return errors.New("賓果卡內不能填寫重複答案")
		}
	}
	p.Answers[index] = answer
	p.Ready = len(p.Answers) == boardCount
	return nil
}

func (r *Room) callAnswer(answer string) error {
	if r.Settings.Mode != "caller" || r.Status != "playing" {
		return errors.New("目前不能公布答案")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" || utf8.RuneCountInString(answer) > 40 {
		return errors.New("公布答案需為 1–40 個字")
	}
	normalized := normalizeAnswer(answer)
	for _, existing := range r.Called {
		if normalizeAnswer(existing) == normalized {
			return errors.New("這個答案已經公布過了")
		}
	}
	r.Called = append(r.Called, answer)
	return nil
}

func (r *Room) answerMatches(answer string) bool {
	normalized := normalizeAnswer(answer)
	for _, called := range r.Called {
		calledNormalized := normalizeAnswer(called)
		if normalized == calledNormalized || r.ApprovedMatches[normalized] == calledNormalized {
			return true
		}
	}
	return false
}

func (r *Room) claim(p *Player, index int) error {
	if (r.Settings.Mode != "caller" && r.Settings.Mode != "turns") || r.Status != "playing" {
		return errors.New("現在不能圈選答案")
	}
	if p.Rank > 0 {
		return errors.New("你已達標，請等待其他玩家")
	}
	answer, ok := p.Answers[index]
	if !ok || index < 0 || index >= r.Settings.Size*r.Settings.Size {
		return errors.New("這一格還沒有答案")
	}
	if _, marked := p.Marks[index]; marked {
		return errors.New("這一格已經圈選")
	}
	if !r.answerMatches(answer) {
		return errors.New("主持人還沒有公布這個答案")
	}
	p.Marks[index] = Signature{Name: answer}
	r.updateProgress(p)
	return nil
}

func (r *Room) activeGroup() string {
	return groupLabel(r.TurnIndex)
}

func (r *Room) groupHasPlayers(group string) bool {
	for _, player := range r.Players {
		if player.Group == group {
			return true
		}
	}
	return false
}

func (r *Room) advanceTurn() {
	for offset := 1; offset <= r.Settings.Groups; offset++ {
		next := (r.TurnIndex + offset) % r.Settings.Groups
		if r.groupHasPlayers(groupLabel(next)) {
			r.TurnIndex = next
			return
		}
	}
}

func (r *Room) initializeTurn() {
	r.TurnIndex = 0
	if r.groupHasPlayers(groupLabel(0)) {
		return
	}
	for index := 1; index < r.Settings.Groups; index++ {
		if r.groupHasPlayers(groupLabel(index)) {
			r.TurnIndex = index
			return
		}
	}
}

func (r *Room) propose(p *Player, index int) error {
	if r.Settings.Mode != "turns" || r.Status != "playing" {
		return errors.New("目前不能提出答案")
	}
	if p.Group != r.activeGroup() {
		return errors.New("還沒輪到你的組別")
	}
	if r.Pending != nil {
		return errors.New("主持人正在審核本組提出的答案")
	}
	answer, ok := p.Answers[index]
	if !ok || index < 0 || index >= r.Settings.Size*r.Settings.Size {
		return errors.New("這一格還沒有答案")
	}
	if r.answerMatches(answer) {
		return errors.New("這個答案已經公布過了")
	}
	r.Pending = &TurnProposal{
		ID: token()[:12], PlayerID: p.ID, PlayerName: p.Name,
		Group: p.Group, Answer: answer,
	}
	return nil
}

func (r *Room) reviewProposal(id, decision string) error {
	if r.Settings.Mode != "turns" || r.Status != "playing" || r.Pending == nil || r.Pending.ID != id {
		return errors.New("找不到這筆待審答案")
	}
	switch decision {
	case "approve":
		answer := r.Pending.Answer
		for _, called := range r.Called {
			if normalizeAnswer(called) == normalizeAnswer(answer) {
				return errors.New("這個答案已經公布過了")
			}
		}
		r.Called = append(r.Called, answer)
		r.Pending = nil
		r.advanceTurn()
	case "reject":
		r.Pending = nil
	default:
		return errors.New("無效的審核決定")
	}
	return nil
}

func (r *Room) skipTurn() error {
	if r.Settings.Mode != "turns" || r.Status != "playing" {
		return errors.New("目前不能跳過組別")
	}
	r.Pending = nil
	r.advanceTurn()
	return nil
}

func (r *Room) requestMatch(p *Player, index int, called string) error {
	if r.Settings.Mode != "turns" || r.Status != "playing" {
		return errors.New("目前不能提出同答案申請")
	}
	answer, ok := p.Answers[index]
	if !ok || index < 0 || index >= r.Settings.Size*r.Settings.Size {
		return errors.New("請選擇自己的有效答案")
	}
	called = strings.TrimSpace(called)
	found := false
	for _, existing := range r.Called {
		if normalizeAnswer(existing) == normalizeAnswer(called) {
			called = existing
			found = true
			break
		}
	}
	if !found {
		return errors.New("請選擇已通過的公布答案")
	}
	if r.answerMatches(answer) {
		return errors.New("這個答案已經可以直接圈選")
	}
	for _, appeal := range r.Appeals {
		if appeal.PlayerID == p.ID && appeal.Index == index && normalizeAnswer(appeal.Called) == normalizeAnswer(called) {
			return errors.New("這筆申請正在等待主持人審核")
		}
	}
	r.Appeals = append(r.Appeals, &MatchAppeal{
		ID: token()[:12], PlayerID: p.ID, PlayerName: p.Name, Group: p.Group,
		Index: index, Answer: answer, Called: called,
	})
	return nil
}

func (r *Room) reviewAppeal(id, decision string) error {
	if r.Settings.Mode != "turns" || r.Status != "playing" {
		return errors.New("目前不能審核同答案申請")
	}
	index := -1
	for appealIndex, appeal := range r.Appeals {
		if appeal.ID == id {
			index = appealIndex
			break
		}
	}
	if index < 0 {
		return errors.New("找不到這筆同答案申請")
	}
	appeal := r.Appeals[index]
	if decision == "approve" {
		r.ApprovedMatches[normalizeAnswer(appeal.Answer)] = normalizeAnswer(appeal.Called)
		remaining := r.Appeals[:0]
		for _, candidate := range r.Appeals {
			if normalizeAnswer(candidate.Answer) != normalizeAnswer(appeal.Answer) ||
				normalizeAnswer(candidate.Called) != normalizeAnswer(appeal.Called) {
				remaining = append(remaining, candidate)
			}
		}
		r.Appeals = remaining
	} else if decision != "reject" {
		return errors.New("無效的審核決定")
	} else {
		r.Appeals = append(r.Appeals[:index], r.Appeals[index+1:]...)
	}
	return nil
}

func (r *Room) updateProgress(p *Player) {
	p.Count = len(p.Marks)
	p.Lines, p.Cells = countLines(r.Settings.Size, p.Marks)
	if p.Lines >= r.Settings.Target && p.Rank == 0 {
		p.Rank = len(r.Winners) + 1
		p.FinishedAt = time.Now().UnixMilli()
		r.Winners = append(r.Winners, p)
		if len(r.Winners) >= r.Settings.Winners {
			r.Status = "finished"
		}
	}
}
func countLines(n int, marks map[int]Signature) (int, []int) {
	lines := [][]int{}
	for i := 0; i < n; i++ {
		row, col := []int{}, []int{}
		for j := 0; j < n; j++ {
			row = append(row, i*n+j)
			col = append(col, j*n+i)
		}
		lines = append(lines, row, col)
	}
	a, b := []int{}, []int{}
	for i := 0; i < n; i++ {
		a = append(a, i*n+i)
		b = append(b, i*n+n-1-i)
	}
	lines = append(lines, a, b)
	count := 0
	cells := map[int]bool{}
	for _, line := range lines {
		ok := true
		for _, c := range line {
			if _, exists := marks[c]; !exists {
				ok = false
				break
			}
		}
		if ok {
			count++
			for _, c := range line {
				cells[c] = true
			}
		}
	}
	result := []int{}
	for c := range cells {
		result = append(result, c)
	}
	sort.Ints(result)
	return count, result
}
func (r *Room) sign(p *Player, index int, s Signature) error {
	if r.Settings.Mode != "social" {
		return errors.New("此模式不使用簽名功能")
	}
	if r.Status != "playing" {
		return errors.New("目前無法簽名，請等待主持人開始或查看遊戲結果")
	}
	if p.Rank > 0 {
		return errors.New("你已達標，請等待其他玩家")
	}
	if index < 0 || index >= len(r.Board) {
		return errors.New("無效的格子")
	}
	if _, ok := p.Marks[index]; ok {
		return errors.New("這一格已經簽名，不能重複提交")
	}
	s.Name = ""
	if s.Ink == "" || len(s.Ink) > 100000 || !strings.HasPrefix(s.Ink, "data:image/png;base64,") {
		return errors.New("請在簽名板上完成簽名後再送出")
	}
	p.Marks[index] = s
	r.updateProgress(p)
	return nil
}
