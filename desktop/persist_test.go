package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 测试一律不碰真实的应用目录。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "aw-chats-")
	if err != nil {
		panic(err)
	}
	chatStoreDir = func() string { return dir }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// 存下 → 换一个全新的应用状态（相当于关掉再打开）→ 读回来：对话、卡片、「这里不对」、试玩都还在。
func TestChatsSurviveRestart(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "out", "玩具剧场")
	s := newState()
	s.productPaths["玩具剧场"] = ws
	s.chats["answers:玩具剧场:fp1"] = &chatSession{Intro: "开场", Msgs: []ChatMsg{{Role: "user", Text: "3 岁"}, {Role: "assistant", Text: "记下了"}},
		replied: true, Chips: []string{"跳过这题"}, Card: &toolCard{Name: "confirm_answers"}, Answers: []qaPair{{Question: "年龄？", Answer: "3 岁"}}}
	s.chats["revise:玩具剧场:v2"] = &chatSession{Intro: "说新需求", Msgs: []ChatMsg{{Role: "user", Text: "加周报"}}}
	s.chats["revise:玩具剧场:v1"] = &chatSession{Intro: "旧的", Msgs: []ChatMsg{{Role: "user", Text: "x"}}, Finished: true}
	s.chats["answers:别的产品:fp"] = &chatSession{Msgs: []ChatMsg{{Role: "user", Text: "不属于这个产品"}}}
	s.notes["玩具剧场"] = []playNote{{ID: 3, Said: "你好", Reply: "嗨", Comment: "要更热情"}}
	s.selected, s.playFP, s.playMsgSeq = "玩具剧场", "planA", 3
	s.playMessages = []PlayMsg{{Role: "user", Text: "你好"}, {Role: "assistant", ID: 3, Text: "嗨", Feedback: true}}
	s.saveProduct("玩具剧场")

	r := newState()
	r.productPaths["玩具剧场"] = ws
	r.mu.Lock()
	r.restoreProduct("玩具剧场")
	r.mu.Unlock()
	a := r.chats["answers:玩具剧场:fp1"]
	if a == nil || len(a.Msgs) != 2 || a.Card == nil || a.Answers[0].Answer != "3 岁" || !a.replied || a.chips()[0] != "跳过这题" {
		t.Fatalf("answers chat not restored: %+v", a)
	}
	if r.chats["revise:玩具剧场:v2"] == nil {
		t.Fatal("revise chat not restored")
	}
	if r.chats["revise:玩具剧场:v1"] != nil || r.chats["answers:别的产品:fp"] != nil {
		t.Fatal("finished chats and other products' chats must not be saved here")
	}
	if len(r.notes["玩具剧场"]) != 1 || r.notes["玩具剧场"][0].Comment != "要更热情" {
		t.Fatalf("notes not restored: %+v", r.notes)
	}

	// 方案没变（开场指纹一致）：试玩接着聊，消息编号接着往上加
	r.mu.Lock()
	r.selected = "玩具剧场"
	r.resumePlay("玩具剧场", "planA")
	r.mu.Unlock()
	if len(r.playMessages) != 2 || r.playMsgSeq != 3 || !r.playMessages[1].Feedback {
		t.Fatalf("play not resumed: %+v seq=%d", r.playMessages, r.playMsgSeq)
	}
}

func TestPlayIsNotResumedWhenThePlanChanged(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "p")
	s := newState()
	s.productPaths["p"] = ws
	s.selected, s.playFP = "p", "old-plan"
	s.playMessages = []PlayMsg{{Role: "user", Text: "你好"}}
	s.saveProduct("p")

	r := newState()
	r.productPaths["p"] = ws
	r.mu.Lock()
	r.restoreProduct("p")
	r.resumePlay("p", "new-plan")
	r.mu.Unlock()
	if len(r.playMessages) != 0 {
		t.Fatalf("a play chat from an older plan must not come back: %+v", r.playMessages)
	}
}

// 什么都没有了（对话确认完、记录带走）就删掉文件，不留空壳。
func TestEmptyProductRemovesTheFile(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "p")
	s := newState()
	s.productPaths["p"] = ws
	s.notes["p"] = []playNote{{ID: 1, Comment: "x"}}
	s.saveProduct("p")
	path := storeFile(ws)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	delete(s.notes, "p")
	s.saveProduct("p")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be removed when nothing is left, err=%v", err)
	}
}
