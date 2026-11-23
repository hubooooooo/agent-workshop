package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// 回答问题、加新需求、试玩的对话，以及「这里不对」的记录，存在引擎里（产品文件夹的 .conversations.json），
// 和网页版共用：网页版聊到一半，打开桌面版能接着聊，反过来也一样。格式见 blueprint/web/conversations.py。
// 哪段对话已经旧了（问题清单变了、需求文档升了版、方案变了）由引擎判断，旧的不会返回。

type wirePart struct {
	Type   string         `json:"type"` // text | data | tool
	Text   string         `json:"text,omitempty"`
	Name   string         `json:"name,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
	Args   map[string]any `json:"args,omitempty"`
	Result any            `json:"result,omitempty"`
}

type wireMsg struct {
	ID    string     `json:"id"`
	Role  string     `json:"role"`
	Parts []wirePart `json:"parts"`
}

type wireSession struct {
	Messages []wireMsg     `json:"messages"`
	Chips    []string      `json:"chips,omitempty"`
	Card     *wireCardEdit `json:"card,omitempty"`
}

// wireCardEdit 卡片上改过、还没确认的内容。
type wireCardEdit struct {
	Answers     []qaPair `json:"answers,omitempty"`
	Title       *string  `json:"title,omitempty"`
	Requirement *string  `json:"requirement,omitempty"`
}

type wireNote struct {
	ID      string `json:"id"`
	Said    string `json:"said"`
	Reply   string `json:"reply"`
	Comment string `json:"comment"`
}

// loadedConversations GET /api/products/{slug}/conversations 的结果。
type loadedConversations struct {
	Answers *wireSession `json:"answers"`
	Revise  *wireSession `json:"revise"`
	Play    *wireSession `json:"play"`
	Notes   []wireNote   `json:"notes"`
}

// newID 消息编号：网页版用随机串，这里也用随机串，两边记下的「这里不对」才对得上同一条回复。
func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "d" + hex.EncodeToString(b)
}

// slugOfChatKey 对话的 key 是「answers:<slug>:<指纹>」「revise:<slug>:v<版本>」。slug 里不会有冒号（见 slugify）。
func slugOfChatKey(key string) string {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

func kindOfChatKey(key string) string {
	return strings.SplitN(key, ":", 2)[0]
}

// -- 对话 ↔ 存档格式 ------------------------------------------------------------

func sessionToWire(cs *chatSession) *wireSession {
	out := &wireSession{}
	for i, m := range cs.Msgs {
		msg := wireMsg{ID: fmt.Sprintf("m%d", i+1), Role: m.Role}
		if m.Text != "" {
			msg.Parts = append(msg.Parts, wirePart{Type: "text", Text: m.Text})
		}
		out.Messages = append(out.Messages, msg)
	}
	if cs.Card != nil {
		part := wirePart{Type: "tool", Name: cs.Card.Name, Args: cs.Card.Args}
		if n := len(out.Messages); n > 0 && out.Messages[n-1].Role == "assistant" {
			out.Messages[n-1].Parts = append(out.Messages[n-1].Parts, part)
		} else {
			out.Messages = append(out.Messages, wireMsg{ID: newID(), Role: "assistant", Parts: []wirePart{part}})
		}
		switch cs.Card.Name {
		case "confirm_answers":
			out.Card = &wireCardEdit{Answers: cs.Answers}
		case "confirm_revision":
			out.Card = &wireCardEdit{Title: &cs.Title, Requirement: &cs.Requirement}
		}
	} else if cs.replied {
		out.Chips = cs.Chips
	}
	return out
}

// sessionFromWire 读回一段对话。网页版存的一条回复可能有好几段文字、一张卡片，这里并成桌面版的样子。
func sessionFromWire(w *wireSession, intro string, openChips []string) *chatSession {
	cs := &chatSession{Intro: intro, OpenChips: openChips, Chips: w.Chips}
	for i, m := range w.Messages {
		var text strings.Builder
		for _, p := range m.Parts {
			switch p.Type {
			case "text":
				text.WriteString(p.Text)
			case "tool":
				if i == len(w.Messages)-1 && p.Result == nil {
					cs.Card = &toolCard{Name: p.Name, Args: p.Args}
				}
			}
		}
		if m.Role == "assistant" {
			cs.replied = true
			if text.Len() == 0 {
				continue
			}
		}
		cs.Msgs = append(cs.Msgs, ChatMsg{Role: m.Role, Text: text.String()})
	}
	if cs.Card != nil {
		cs.fillCard()
		if e := w.Card; e != nil {
			if len(e.Answers) > 0 {
				cs.Answers = e.Answers
			}
			if e.Title != nil {
				cs.Title = *e.Title
			}
			if e.Requirement != nil {
				cs.Requirement = *e.Requirement
			}
		}
		cs.Chips = nil
	}
	return cs
}

// playToWire 试玩：桌面版把模拟的每一步单独存一行，存档格式里一轮回复是一条消息（步骤 + 文字 + 「这里不对」）。
func playToWire(msgs []PlayMsg) *wireSession {
	out := &wireSession{}
	var cur *wireMsg
	flush := func() {
		if cur != nil {
			if cur.ID == "" {
				cur.ID = newID()
			}
			out.Messages = append(out.Messages, *cur)
			cur = nil
		}
	}
	for _, m := range msgs {
		switch m.Role {
		case "user":
			flush()
			out.Messages = append(out.Messages, wireMsg{ID: newID(), Role: "user", Parts: []wirePart{{Type: "text", Text: m.Text}}})
		case "step":
			if cur == nil {
				cur = &wireMsg{Role: "assistant"}
			}
			data := map[string]any{}
			for k, v := range m.Step {
				data[k] = v
			}
			cur.Parts = append(cur.Parts, wirePart{Type: "data", Name: "play_step", Data: data})
		default:
			if cur == nil {
				cur = &wireMsg{Role: "assistant"}
			}
			cur.ID = m.ID
			if m.Text != "" {
				cur.Parts = append(cur.Parts, wirePart{Type: "text", Text: m.Text})
			}
			if m.Feedback {
				cur.Parts = append(cur.Parts, wirePart{Type: "data", Name: "play_feedback", Data: map[string]any{}})
			}
			flush()
		}
	}
	flush()
	return out
}

func playFromWire(w *wireSession) []PlayMsg {
	var out []PlayMsg
	for _, m := range w.Messages {
		if m.Role == "user" {
			var text strings.Builder
			for _, p := range m.Parts {
				if p.Type == "text" {
					text.WriteString(p.Text)
				}
			}
			out = append(out, PlayMsg{Role: "user", Text: text.String()})
			continue
		}
		reply := PlayMsg{Role: "assistant", ID: m.ID}
		var text strings.Builder
		for _, p := range m.Parts {
			switch {
			case p.Type == "data" && p.Name == "play_step":
				step := map[string]string{}
				for k, v := range p.Data {
					if s, ok := v.(string); ok {
						step[k] = s
					}
				}
				out = append(out, PlayMsg{Role: "step", Step: step})
			case p.Type == "data" && p.Name == "play_feedback":
				reply.Feedback = true
			case p.Type == "text":
				text.WriteString(p.Text)
			}
		}
		reply.Text = text.String()
		if reply.ID == "" {
			reply.ID = newID()
		}
		out = append(out, reply)
	}
	return out
}

func notesToWire(notes []playNote) []wireNote {
	out := make([]wireNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, wireNote{ID: n.ID, Said: n.Said, Reply: n.Reply, Comment: n.Comment})
	}
	return out
}

func notesFromWire(notes []wireNote) []playNote {
	out := make([]playNote, 0, len(notes))
	for _, n := range notes {
		out = append(out, playNote{ID: n.ID, Said: n.Said, Reply: n.Reply, Comment: n.Comment})
	}
	return out
}

// -- 读写 ------------------------------------------------------------------------

func (s *AppState) postConversation(slug, kind string, body any) {
	s.mu.Lock()
	base := s.engineURL
	s.mu.Unlock()
	if base == "" || slug == "" {
		return
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	_, _ = postJSONData(base, "/api/products/"+url.PathEscape(slug)+"/conversations/"+kind, string(raw))
}

// saveChat 存（或删掉）回答问题 / 加新需求这段对话。没说过话、已经确认完成的不留。
func (s *AppState) saveChat(key string) {
	slug, kind := slugOfChatKey(key), kindOfChatKey(key)
	s.mu.Lock()
	var session *wireSession
	if cs := s.chats[key]; cs != nil && !cs.Finished && cs.hasUser() {
		session = sessionToWire(cs)
	}
	s.mu.Unlock()
	s.postConversation(slug, kind, map[string]any{"session": session})
}

// savePlay 存试玩。试玩的状态只属于当前打开的产品。
func (s *AppState) savePlay(slug string) {
	s.mu.Lock()
	if s.selected != slug || s.playStreaming {
		s.mu.Unlock()
		return
	}
	var session *wireSession
	if len(s.playMessages) > 0 {
		session = playToWire(s.playMessages)
	}
	s.mu.Unlock()
	s.postConversation(slug, "play", map[string]any{"session": session})
}

func (s *AppState) saveNotes(slug string) {
	s.mu.Lock()
	notes := notesToWire(s.notes[slug])
	s.mu.Unlock()
	s.postConversation(slug, "notes", map[string]any{"notes": notes})
}

// loadConversations 打开产品时读回存下的对话（每次打开应用只读一次；之后以界面上的为准）。
func (s *AppState) loadConversations(slug string) {
	s.mu.Lock()
	if s.restored[slug] {
		s.mu.Unlock()
		return
	}
	s.restored[slug] = true
	base := s.engineURL
	s.mu.Unlock()
	data, err := apiGet(base, "/api/products/"+url.PathEscape(slug)+"/conversations")
	var got loadedConversations
	if err == nil {
		raw, _ := json.Marshal(data)
		err = json.Unmarshal(raw, &got)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		delete(s.restored, slug) // 下次打开再试
		return
	}
	s.convPending[slug] = &got
	s.notes[slug] = notesFromWire(got.Notes) // 引擎里的是最新的：每次改都会存过去
	// 这次打开前建过的对话换成存下的（另一个界面可能接着聊过、或者重新开始了），正在回复的不动
	for key, cs := range s.chats {
		if slugOfChatKey(key) != slug || cs.Streaming || cs.CardBusy || cs.Finished {
			continue
		}
		fresh := &chatSession{Intro: cs.Intro, OpenChips: cs.OpenChips}
		if w := got.session(kindOfChatKey(key)); w != nil {
			fresh = sessionFromWire(w, cs.Intro, cs.OpenChips)
			got.take(kindOfChatKey(key))
		}
		fresh.Draft, fresh.InputGen, fresh.gen = cs.Draft, cs.InputGen+1, cs.gen+1
		s.chats[key] = fresh
	}
	if s.selected == slug && s.playIntro != "" {
		s.resumePlay(slug)
	}
	s.invalidate()
}

func (l *loadedConversations) session(kind string) *wireSession {
	switch kind {
	case "answers":
		return l.Answers
	case "revise":
		return l.Revise
	case "play":
		return l.Play
	}
	return nil
}

// take 取走一段（只恢复一次）。
func (l *loadedConversations) take(kind string) {
	switch kind {
	case "answers":
		l.Answers = nil
	case "revise":
		l.Revise = nil
	case "play":
		l.Play = nil
	}
}

// pendingSession 新建对话时看看有没有存下的可以接上。调用方必须已持有 s.mu。
func (s *AppState) pendingSession(key string) *wireSession {
	p := s.convPending[slugOfChatKey(key)]
	if p == nil {
		return nil
	}
	w := p.session(kindOfChatKey(key))
	p.take(kindOfChatKey(key))
	return w
}

// resumePlay 试玩开场到了、上次的试玩还在：接着聊。调用方必须已持有 s.mu。
func (s *AppState) resumePlay(slug string) {
	p := s.convPending[slug]
	if p == nil || p.Play == nil || len(s.playMessages) > 0 {
		return
	}
	s.playMessages = playFromWire(p.Play)
	p.Play = nil
	s.playChips = nil // 已经聊过了，开场的场景不再给
}
