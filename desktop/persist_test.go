package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeConversationEngine 只实现对话存档的两个接口，原样存、原样还（作废判断是引擎的事，另有 Python 测试）。
func fakeConversationEngine(t *testing.T) (*httptest.Server, map[string]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	store := map[string]map[string]any{} // slug → {answers, revise, play, notes}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/products/"), "/")
		slug := parts[0]
		if store[slug] == nil {
			store[slug] = map[string]any{}
		}
		if r.Method == http.MethodGet {
			out := map[string]any{"notes": []any{}, "answers": nil, "revise": nil, "play": nil}
			for k, v := range store[slug] {
				out[k] = v
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if kind := parts[2]; kind == "notes" {
			store[slug]["notes"] = body["notes"]
		} else {
			store[slug][kind] = body["session"]
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(srv.Close)
	return srv, store
}

// 存下 → 换一个全新的应用状态（相当于关掉再打开，或者换成网页版）→ 读回来：对话、卡片、「这里不对」、试玩都还在。
func TestConversationsRoundTripThroughTheEngine(t *testing.T) {
	srv, store := fakeConversationEngine(t)
	s := newState()
	s.engineURL = srv.URL
	ak, rk := "answers:玩具剧场:fp1", "revise:玩具剧场:v2"
	s.chats[ak] = &chatSession{Intro: "开场", Msgs: []ChatMsg{{Role: "user", Text: "3 岁"}, {Role: "assistant", Text: "记下了"}},
		replied: true, Card: &toolCard{Name: "confirm_answers"}, Answers: []qaPair{{Question: "年龄？", Answer: "3 岁"}}}
	s.chats[rk] = &chatSession{Intro: "说新需求", Msgs: []ChatMsg{{Role: "user", Text: "加周报"}, {Role: "assistant", Text: "按周还是按月？"}},
		replied: true, Chips: []string{"按周"}}
	s.notes["玩具剧场"] = []playNote{{ID: "m3", Said: "你好", Reply: "嗨", Comment: "要更热情"}}
	s.selected = "玩具剧场"
	s.playMessages = []PlayMsg{{Role: "user", Text: "你好"},
		{Role: "step", Step: map[string]string{"tool": "search"}},
		{Role: "assistant", ID: "m3", Text: "嗨", Feedback: true}}
	s.saveChat(ak)
	s.saveChat(rk)
	s.saveNotes("玩具剧场")
	s.savePlay("玩具剧场")
	if len(store["玩具剧场"]) != 4 {
		t.Fatalf("expected 4 entries stored, got %v", store["玩具剧场"])
	}

	r := newState()
	r.engineURL = srv.URL
	r.loadConversations("玩具剧场")
	r.mu.Lock()
	a := r.chatFor(ak, "answers:玩具剧场:", "开场", nil)
	rv := r.chatFor(rk, "revise:玩具剧场:", "说新需求", nil)
	r.selected = "玩具剧场"
	r.resumePlay("玩具剧场")
	r.mu.Unlock()
	if len(a.Msgs) != 2 || a.Card == nil || a.Card.Name != "confirm_answers" || a.Answers[0].Answer != "3 岁" || !a.replied {
		t.Fatalf("answers chat not restored: %+v", a)
	}
	if len(rv.Msgs) != 2 || rv.chips()[0] != "按周" {
		t.Fatalf("revise chat not restored: %+v chips=%v", rv, rv.chips())
	}
	if n := r.notes["玩具剧场"]; len(n) != 1 || n[0].ID != "m3" || n[0].Comment != "要更热情" {
		t.Fatalf("notes not restored: %+v", n)
	}
	if p := r.playMessages; len(p) != 3 || p[1].Role != "step" || p[1].Step["tool"] != "search" || p[2].ID != "m3" || !p[2].Feedback {
		t.Fatalf("play not restored: %+v", p)
	}
}

// 确认完成、没说过话的对话不留；试玩清空也删掉。
func TestFinishedAndEmptyConversationsAreDeleted(t *testing.T) {
	srv, store := fakeConversationEngine(t)
	s := newState()
	s.engineURL = srv.URL
	key := "revise:p:v1"
	s.chats[key] = &chatSession{Msgs: []ChatMsg{{Role: "user", Text: "x"}}}
	s.saveChat(key)
	if store["p"]["revise"] == nil {
		t.Fatal("an open chat should be stored")
	}
	s.chats[key].Finished = true
	s.saveChat(key)
	if store["p"]["revise"] != nil {
		t.Fatalf("a finished chat must be deleted, got %v", store["p"]["revise"])
	}
	s.selected = "p"
	s.playMessages = nil
	store["p"]["play"] = map[string]any{"messages": []any{}}
	s.savePlay("p")
	if store["p"]["play"] != nil {
		t.Fatal("an empty play chat must be deleted")
	}
}

// 网页版存的样子：一条回复好几段文字、卡片是工具调用，卡片上改过的内容另存。
func TestWebShapedSessionsAreRead(t *testing.T) {
	title, req := "加周报", "每周一发上周的总结"
	w := &wireSession{Messages: []wireMsg{
		{ID: "u1", Role: "user", Parts: []wirePart{{Type: "text", Text: "想加个周报"}}},
		{ID: "a1", Role: "assistant", Parts: []wirePart{
			{Type: "text", Text: "好的，"}, {Type: "text", Text: "草稿如下"},
			{Type: "tool", Name: "confirm_revision", Args: map[string]any{"title": "周报", "requirement": "发周报"}},
		}},
	}, Card: &wireCardEdit{Title: &title, Requirement: &req}}
	cs := sessionFromWire(w, "开场", []string{"开场选项"})
	if len(cs.Msgs) != 2 || cs.Msgs[1].Text != "好的，草稿如下" {
		t.Fatalf("text parts should be joined: %+v", cs.Msgs)
	}
	if cs.Card == nil || cs.Card.Name != "confirm_revision" || cs.Title != title || cs.Requirement != req {
		t.Fatalf("card edits not applied: card=%+v title=%q req=%q", cs.Card, cs.Title, cs.Requirement)
	}
	// 再写回去还是一张卡片挂在最后一条回复上
	back := sessionToWire(cs)
	last := back.Messages[len(back.Messages)-1]
	if last.Role != "assistant" || last.Parts[len(last.Parts)-1].Type != "tool" || *back.Card.Title != title {
		t.Fatalf("card not written back: %+v", back)
	}

	// 已经有结果的工具调用（网页版确认过的）不再当成待确认的卡片
	w.Messages[1].Parts[2].Result = map[string]any{"ok": true}
	if cs := sessionFromWire(w, "", nil); cs.Card != nil {
		t.Fatal("a tool part with a result is not a pending card")
	}
}

// 打开产品时引擎里的是最新的：另一个界面重新开始过，这边没在回复的对话也跟着清空。
func TestReopeningAProductPicksUpTheOtherUIsChanges(t *testing.T) {
	srv, _ := fakeConversationEngine(t)
	s := newState()
	s.engineURL = srv.URL
	key := "answers:p:fp"
	s.chats[key] = &chatSession{Intro: "开场", Msgs: []ChatMsg{{Role: "user", Text: "旧的"}}, Draft: "没发出去的"}
	s.loadConversations("p") // 引擎里没有：另一边已经重新开始了
	s.mu.Lock()
	cs := s.chats[key]
	s.mu.Unlock()
	if cs.hasUser() || cs.Intro != "开场" || cs.Draft != "没发出去的" {
		t.Fatalf("chat should be reset to the intro, keeping the draft: %+v", cs)
	}
}

// 回复到一半点「停止」：断开这条流，已经出来的部分留着，不算出错，并且存下来。
func TestStopKeepsThePartialReply(t *testing.T) {
	_, store := fakeConversationEngine(t)
	release := make(chan struct{})
	var saved sync.WaitGroup
	saved.Add(1)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat/answers/p", func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		_, _ = io.WriteString(w, `{"type":"text","delta":"先说一半"}`+"\n")
		fl.Flush()
		select { // 一直不说完，直到客户端断开
		case <-r.Context().Done():
		case <-release:
		}
	})
	mux.HandleFunc("/api/products/p/conversations/answers", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		store["p"] = map[string]any{"answers": body["session"]}
		_, _ = io.WriteString(w, `{"ok":true}`)
		saved.Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer close(release)

	s := newState()
	s.engineURL = srv.URL
	key := "answers:p:fp"
	s.chats[key] = &chatSession{Intro: "开场"}
	done := make(chan struct{})
	go func() { s.chatSend(key, "/api/chat/answers/p", "你好"); close(done) }()
	for i := 0; ; i++ { // 等第一段文字到
		s.mu.Lock()
		n := len(s.chats[key].Msgs)
		s.mu.Unlock()
		if n == 2 {
			break
		}
		if i > 500 {
			t.Fatal("partial reply never arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.chatStop(key)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not end the stream")
	}
	saved.Wait()
	cs := s.chats[key]
	if cs.Streaming || cs.Err != "" || cs.Msgs[1].Text != "先说一半" {
		t.Fatalf("after stop: streaming=%v err=%q msgs=%+v", cs.Streaming, cs.Err, cs.Msgs)
	}
	if store["p"]["answers"] == nil {
		t.Fatal("the stopped chat should still be saved")
	}
}
