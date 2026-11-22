package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// composerHarness 模拟一个对话页：草稿和发送次数放在外面，像 app 的状态一样每帧复制进来。
type composerHarness struct {
	draft string
	gen   int
	busy  bool
	sent  []string
}

func (h *composerHarness) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1)
		draft := h.draft
		if text := chatComposer(c, h.gen, &draft, composerProps{Placeholder: "说点什么", Busy: h.busy, BusyText: "正在回复…"}); text != "" {
			h.sent = append(h.sent, text)
			h.gen++
			draft = ""
		}
		h.draft = draft
	})
}

func TestComposerEnterSendsAndClears(t *testing.T) {
	h := &composerHarness{}
	tt := ui.NewTester(h.view, 900, 600)
	tt.Type("明天有面包吗")
	tt.Frame()
	if h.draft != "明天有面包吗" {
		t.Fatalf("typed text should be written back, got %q", h.draft)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(h.sent) != 1 || h.sent[0] != "明天有面包吗" {
		t.Fatalf("Enter should send once, sent=%q", h.sent)
	}
	if h.draft != "" {
		t.Fatalf("draft should be cleared after sending, got %q", h.draft)
	}
	tt.Type("再来一个")
	tt.Frame()
	if h.draft != "再来一个" {
		t.Fatalf("the rebuilt input should start empty and keep focus, got %q", h.draft)
	}
}

func TestComposerDoesNotSendEmptyOrWhileBusy(t *testing.T) {
	h := &composerHarness{}
	tt := ui.NewTester(h.view, 900, 600)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(h.sent) != 0 {
		t.Fatalf("empty input must not send, sent=%q", h.sent)
	}
	h.busy = true
	tt.Type("等一下")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(h.sent) != 0 {
		t.Fatalf("must not send while the AI is replying, sent=%q", h.sent)
	}
	if h.draft != "等一下" {
		t.Fatalf("the draft should be kept while busy, got %q", h.draft)
	}
	if !tt.HasText("正在回复…") {
		t.Fatal("busy text should replace the shortcut hint")
	}
}

// 点右下角的发送按钮也要能发出去（不只是 Enter）。
func TestComposerSendButtonSends(t *testing.T) {
	h := &composerHarness{}
	tt := ui.NewTester(h.view, 900, 600)
	tt.Type("点按钮发送")
	tt.Frame()
	clickSend(t, tt)
	tt.Frame()
	if len(h.sent) != 1 || h.sent[0] != "点按钮发送" {
		t.Fatalf("clicking the send button should send, sent=%q", h.sent)
	}
	if h.draft != "" {
		t.Fatalf("draft should be cleared after sending, got %q", h.draft)
	}
}

// clickSend 点输入框右下角的圆形发送按钮：和左下角的提示在同一行，靠卡片右边。
func clickSend(t *testing.T, tt *ui.Tester) {
	t.Helper()
	hint, ok := tt.Find("Enter 发送")
	if !ok {
		t.Fatal("composer hint not found")
	}
	// 卡片最宽 chatWidth，在 900 宽的窗口里居中（左右各留 70），内边距 16，按钮 32 宽。
	x := float32(900-(900-chatWidth)/2) - 16 - 16
	tt.ClickAt(x, hint.Y+hint.H/2)
}
