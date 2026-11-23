package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// chatSession 是「回答问题」「加新需求」这类对话的一段：和网页版 chat-runtime 一样，
// 每次把整段对话（含开场白）发给引擎，引擎无状态地重放；回复里可能带一张待确认的卡片，
// 确认之后这段对话就结束了（finished），输入框换成一句「这段对话已经完成。」。
type chatSession struct {
	Intro     string    // 开场白：界面自己加的第一条 assistant 消息，不调引擎
	Msgs      []ChatMsg // 开场白之后的对话
	OpenChips []string  // 第一次回复前的快捷回复
	Chips     []string  // 最近一次回复带的快捷回复
	replied   bool
	Streaming bool
	Err       string
	Card      *toolCard // 待确认的卡片（confirm_answers / confirm_revision）
	Finished  bool      // 卡片已确认
	Started   *JobInfo  // 确认后启动的任务
	StartedAs string    // 确认后卡片的标题
	Draft     string
	InputGen  int
	gen       int    // 重新开始就 +1：旧的流式回复对不上号就丢掉
	cancel    func() // 正在回复：点「停止」时调

	// 卡片里可以改的内容
	Answers     []qaPair
	Title       string
	Requirement string
	CardBusy    bool
	CardErr     string
	CardGen     int // 卡片输入框按它重建
}

type toolCard struct {
	Name string
	Args map[string]any
}

type qaPair struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// chips 当前该显示的快捷回复：还没回复过用开场的，回复过用最近一次的。
func (cs *chatSession) chips() []string {
	if !cs.replied {
		return cs.OpenChips
	}
	return cs.Chips
}

// hasUser 是否已经说过话（重新开始按钮只在说过话后出现）。
func (cs *chatSession) hasUser() bool {
	for _, m := range cs.Msgs {
		if m.Role == "user" {
			return true
		}
	}
	return false
}

// chatFor 取一段对话，没有就按给的开场白新建。prefix 相同的旧对话一并丢掉
// （问题清单或版本变了，旧的回答对不上了，和网页版清 localStorage 一样）。调用方必须已持有 s.mu。
func (s *AppState) chatFor(key, prefix, intro string, chips []string) *chatSession {
	if cs, ok := s.chats[key]; ok {
		return cs
	}
	for k := range s.chats {
		if strings.HasPrefix(k, prefix) {
			delete(s.chats, k)
		}
	}
	cs := &chatSession{Intro: intro, OpenChips: chips}
	if w := s.pendingSession(key); w != nil { // 存在引擎里的上次的对话：接着聊
		cs = sessionFromWire(w, intro, chips)
	}
	s.chats[key] = cs
	return cs
}

// chatSend 发一句话：整段对话发到 endpoint，事件流逐条写进这段对话。
func (s *AppState) chatSend(key, endpoint, text string) {
	s.mu.Lock()
	cs := s.chats[key]
	text = strings.TrimSpace(text)
	if cs == nil || cs.Streaming || cs.Finished || text == "" {
		s.mu.Unlock()
		return
	}
	cs.Msgs = append(cs.Msgs, ChatMsg{Role: "user", Text: text})
	cs.Streaming, cs.Err, cs.Draft, cs.Card = true, "", "", nil
	cs.InputGen++
	cs.gen++
	gen := cs.gen
	history := append([]ChatMsg{{Role: "assistant", Text: cs.Intro}}, cs.Msgs...)
	base := s.engineURL
	ctx, cancel := context.WithCancel(context.Background())
	cs.cancel = cancel
	s.mu.Unlock()
	s.invalidate()
	defer cancel()

	err := streamChat(ctx, base, endpoint, history, func(ev ChatEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.chats[key] != cs || cs.gen != gen {
			return
		}
		switch ev.Type {
		case "text":
			if n := len(cs.Msgs); n == 0 || cs.Msgs[n-1].Role != "assistant" {
				cs.Msgs = append(cs.Msgs, ChatMsg{Role: "assistant"})
			}
			cs.Msgs[len(cs.Msgs)-1].Text += ev.Delta
		case "tool":
			cs.Card = &toolCard{Name: ev.Name, Args: ev.Args}
			cs.fillCard()
		case "suggestions":
			cs.Chips = ev.Items
		case "error":
			cs.Err = ev.Message
		}
		s.invalidate()
	})
	s.mu.Lock()
	if s.chats[key] == cs && cs.gen == gen {
		cs.Streaming = false
		cs.replied = true
		cs.cancel = nil
		if err != nil && cs.Err == "" && ctx.Err() == nil { // 自己点了「停止」不算出错
			cs.Err = err.Error()
		}
		if cs.Card != nil {
			cs.Chips = nil
		}
	}
	s.mu.Unlock()
	s.invalidate()
	s.saveChat(key)
}

// chatStop 回复进行中点「停止」：断开这条流，已经出来的部分留着。
func (s *AppState) chatStop(key string) {
	s.mu.Lock()
	var cancel func()
	if cs := s.chats[key]; cs != nil {
		cancel = cs.cancel
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// fillCard 把卡片的参数复制成可以改的字段。
func (cs *chatSession) fillCard() {
	cs.CardErr, cs.CardGen = "", cs.CardGen+1
	switch cs.Card.Name {
	case "confirm_answers":
		cs.Answers = nil
		if raw, err := json.Marshal(cs.Card.Args["answers"]); err == nil {
			_ = json.Unmarshal(raw, &cs.Answers)
		}
	case "confirm_revision":
		cs.Title, _ = cs.Card.Args["title"].(string)
		cs.Requirement, _ = cs.Card.Args["requirement"].(string)
	}
}

// chatRegenerate 「重新生成」：去掉最后一条回复，把上一句话重新发一次。
func (s *AppState) chatRegenerate(key, endpoint string) {
	s.mu.Lock()
	cs := s.chats[key]
	if cs == nil || cs.Streaming || cs.Finished {
		s.mu.Unlock()
		return
	}
	n := len(cs.Msgs)
	for n > 0 && cs.Msgs[n-1].Role != "user" {
		n--
	}
	if n == 0 {
		s.mu.Unlock()
		return
	}
	text := cs.Msgs[n-1].Text
	cs.Msgs, cs.Card = cs.Msgs[:n-1], nil
	s.mu.Unlock()
	s.chatSend(key, endpoint, text)
}

// chatEditResend 「修改」后重新发送：第 idx 条及之后的对话丢掉，改过的话重新发。
func (s *AppState) chatEditResend(key, endpoint string, idx int, text string) {
	s.mu.Lock()
	cs := s.chats[key]
	if cs == nil || cs.Streaming || cs.Finished || idx < 0 || idx >= len(cs.Msgs) {
		s.mu.Unlock()
		return
	}
	cs.Msgs, cs.Card, cs.Chips = cs.Msgs[:idx], nil, nil
	cs.replied = idx > 0
	s.mu.Unlock()
	s.chatSend(key, endpoint, text)
}

// chatRestart 清空这段对话，回到开场白。
func (s *AppState) chatRestart(key string) {
	s.mu.Lock()
	if cs := s.chats[key]; cs != nil && !cs.Streaming {
		fresh := &chatSession{Intro: cs.Intro, OpenChips: cs.OpenChips, gen: cs.gen + 1, InputGen: cs.InputGen + 1}
		s.chats[key] = fresh
	}
	s.mu.Unlock()
	s.invalidate()
	go s.saveChat(key)
}

// confirmAnswers 「保存并重新推导方案」：回答写进需求文档，启动推导任务。
func (s *AppState) confirmAnswers(key, slug string) {
	s.mu.Lock()
	cs := s.chats[key]
	if cs == nil || cs.CardBusy || cs.Finished {
		s.mu.Unlock()
		return
	}
	cs.CardBusy, cs.CardErr = true, ""
	body, _ := json.Marshal(map[string]any{"answers": cs.Answers})
	base := s.engineURL
	s.mu.Unlock()
	s.invalidate()
	data, err := postJSONData(base, "/api/products/"+url.PathEscape(slug)+"/answers", string(body))
	s.finishCard(key, cs, slug, data, err, "回答已写进需求文档")
}

// confirmRevision 「升到 Vn 并重新推导」：需求文档升一版，按新需求重新推导。
func (s *AppState) confirmRevision(key, slug string, next int) {
	s.mu.Lock()
	cs := s.chats[key]
	if cs == nil || cs.CardBusy || cs.Finished {
		s.mu.Unlock()
		return
	}
	title, req := strings.TrimSpace(cs.Title), strings.TrimSpace(cs.Requirement)
	if req == "" {
		cs.CardErr = "新需求不能为空。"
		s.mu.Unlock()
		s.invalidate()
		return
	}
	cs.CardBusy, cs.CardErr = true, ""
	body, _ := json.Marshal(map[string]string{"title": title, "requirement": req})
	base := s.engineURL
	s.mu.Unlock()
	s.invalidate()
	data, err := postJSONData(base, "/api/products/"+url.PathEscape(slug)+"/revise", string(body))
	label := fmt.Sprintf("已升到 V%d", next)
	if title != "" {
		label += "：" + title
	}
	s.finishCard(key, cs, slug, data, err, label)
}

func (s *AppState) finishCard(key string, cs *chatSession, slug string, data map[string]any, err error, label string) {
	s.mu.Lock()
	cs.CardBusy = false
	if err != nil {
		cs.CardErr = err.Error()
		s.mu.Unlock()
		s.invalidate()
		return
	}
	cs.Finished, cs.StartedAs = true, label
	if raw, ok := data["job"]; ok && raw != nil {
		if job, jerr := parseJob(raw); jerr == nil {
			cs.Started = job
		}
	}
	s.mu.Unlock()
	go s.saveChat(key)  // 确认过的对话不再留
	s.fetchDetail(slug) // 拿到正在跑的任务，接着轮询进度
}

// -- 试玩里的「这里不对」 ------------------------------------------------------

// playNote 一条「这里不对」：我说了什么、它回了什么、我希望怎样。
type playNote struct {
	ID      string
	Said    string
	Reply   string
	Comment string
}

// saveNote 记下（或改写）某条回复下的批注。
func (s *AppState) saveNote(slug, msgID, comment string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return
	}
	said, reply := "", ""
	for i, m := range s.playMessages {
		if m.ID == msgID && m.Role == "assistant" {
			reply = m.Text
			for j := i - 1; j >= 0; j-- {
				if s.playMessages[j].Role == "user" {
					said = s.playMessages[j].Text
					break
				}
			}
		}
	}
	notes := s.notes[slug][:0:0]
	for _, n := range s.notes[slug] {
		if n.ID != msgID {
			notes = append(notes, n)
		}
	}
	s.notes[slug] = append(notes, playNote{ID: msgID, Said: said, Reply: reply, Comment: comment})
	s.noteOpen, s.noteDraft = "", ""
	s.invalidate()
	go s.saveNotes(slug)
}

func (s *AppState) removeNote(slug, msgID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	notes := s.notes[slug][:0:0]
	for _, n := range s.notes[slug] {
		if n.ID != msgID {
			notes = append(notes, n)
		}
	}
	s.notes[slug] = notes
	s.invalidate()
	go s.saveNotes(slug)
}

// carryNotes 「带去加新需求 →」：把记下的几处整理成一段草稿，放进加新需求的输入框。
func (s *AppState) carryNotes(slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	notes := s.notes[slug]
	if len(notes) == 0 {
		return
	}
	lines := make([]string, 0, len(notes))
	for i, n := range notes {
		lines = append(lines, fmt.Sprintf("%d. 我说「%s」，它回「%s」。我希望：%s", i+1, n.Said, n.Reply, n.Comment))
	}
	s.reviseSeed[slug] = "试玩时发现这些地方和我想的不一样：\n" + strings.Join(lines, "\n")
	delete(s.notes, slug)
	s.productTab = "revise"
	s.invalidate()
	go s.saveNotes(slug)
}

// playEditResend 试玩里「修改」后重新发送：这句及之后的动作和回复丢掉，改过的话重新发。
func (s *AppState) playEditResend(idx int, text string) {
	s.mu.Lock()
	if s.playStreaming || idx < 0 || idx >= len(s.playMessages) {
		s.mu.Unlock()
		return
	}
	s.playMessages = s.playMessages[:idx]
	s.mu.Unlock()
	s.sendPlay(text)
}

// chatIntakeEditResend 新建产品里「修改」后重新发送。
func (s *AppState) chatIntakeEditResend(idx int, text string) {
	s.mu.Lock()
	if s.chatStreaming || idx < 0 || idx >= len(s.chatMessages) {
		s.mu.Unlock()
		return
	}
	s.chatMessages = s.chatMessages[:idx]
	s.chatConfirm, s.chatSuggest = nil, nil
	s.mu.Unlock()
	s.sendChat(text)
}

// regeneratePlay 试玩「重新生成」：去掉最后一轮的动作卡和回复，把上一句话重新发一次。
func (s *AppState) regeneratePlay() {
	s.mu.Lock()
	if s.playStreaming {
		s.mu.Unlock()
		return
	}
	n := len(s.playMessages)
	for n > 0 && s.playMessages[n-1].Role != "user" {
		n--
	}
	if n == 0 {
		s.mu.Unlock()
		return
	}
	text := s.playMessages[n-1].Text
	s.playMessages = s.playMessages[:n-1]
	s.mu.Unlock()
	s.sendPlay(text)
}

// restartPlay 试玩「重新开始」：清空对话，记下的「这里不对」留着。
func (s *AppState) restartPlay(slug string) {
	s.mu.Lock()
	if s.playStreaming {
		s.mu.Unlock()
		return
	}
	s.playStarted, s.playIntro, s.playMessages, s.playErr, s.playDraft = false, "", nil, "", ""
	s.playInputGen++
	s.playGen++
	s.noteOpen = ""
	s.mu.Unlock()
	s.savePlay(slug) // 对话空了：引擎那边也删掉
	s.startPlay(slug)
}

// -- 版本历史 ----------------------------------------------------------------

// History 是 GET /api/products/{slug}/history。
type History struct {
	Current   int              `json:"current"`
	Versions  []HistoryVersion `json:"versions"`
	Log       []HistoryEntry   `json:"log"`
	Snapshots []int            `json:"snapshots"`
}

type HistoryVersion struct {
	Version int    `json:"version"`
	Date    string `json:"date"`
	Change  string `json:"change"`
	Source  string `json:"source"`
}

type HistoryEntry struct {
	Date    string   `json:"date"`
	Version *int     `json:"version"`
	Title   string   `json:"title"`
	Lines   []string `json:"lines"`
}

// PrdDoc 是某一版的需求文档。
type PrdDoc struct {
	Version int    `json:"version"`
	Text    string `json:"text"`
	Current bool   `json:"current"`
}

// loadHistory 拉版本历史。key 是「产品:版本:更新时间」，变了才重拉。
func (s *AppState) loadHistory(slug, key string) {
	s.mu.Lock()
	if s.historyFor == key {
		s.mu.Unlock()
		return
	}
	s.historyFor = key
	base := s.engineURL
	s.mu.Unlock()
	data, err := apiGet(base, "/api/products/"+url.PathEscape(slug)+"/history")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected != slug {
		return
	}
	if err != nil {
		s.history, s.historyErr = nil, err.Error()
	} else {
		var h History
		raw, _ := json.Marshal(data)
		_ = json.Unmarshal(raw, &h)
		sort.SliceStable(h.Versions, func(i, j int) bool { return h.Versions[i].Version < h.Versions[j].Version })
		s.history, s.historyErr = &h, ""
	}
	s.invalidate()
}

// openPrd 打开某一版需求文档；version 为 0 表示当前版。
func (s *AppState) openPrd(slug string, version int) {
	s.mu.Lock()
	s.prdDoc, s.prdDocErr, s.prdDocOpen = nil, "", true
	base := s.engineURL
	s.mu.Unlock()
	s.invalidate()
	path := "/api/products/" + url.PathEscape(slug) + "/prd"
	if version > 0 {
		path += fmt.Sprintf("?v=%d", version)
	}
	data, err := apiGet(base, path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.prdDocErr = err.Error()
	} else {
		var d PrdDoc
		raw, _ := json.Marshal(data)
		_ = json.Unmarshal(raw, &d)
		s.prdDoc = &d
	}
	s.invalidate()
}

// fingerprint 问题清单的指纹：清单变了（重新推导出新问题），回答问题就换一段新对话。
func fingerprint(items []string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.Join(items, "\u0000")))
	return strconv.FormatUint(uint64(h.Sum32()), 36)
}
