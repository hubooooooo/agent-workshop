package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// chatWidth 是对话区和输入框共用的最大宽度：两者对齐成一列，窗口再宽也不铺满。
const chatWidth = 760

// composerProps 配置对话页底部的输入框。
type composerProps struct {
	Placeholder string
	// Busy：AI 正在回复。可以继续打字，但不能发送。
	Busy bool
	// BusyText 在 Busy 时代替快捷键提示显示在左下角。
	BusyText string
	// OnStop 不为空时，Busy 期间发送按钮变成「停止」，点了调它（和网页版一样）。
	OnStop func()
}

// chatComposer 是对话页底部的输入框：全宽居中的圆角卡片，文字多了自动增高（最多 8 行），
// 发送按钮在卡片右下角。Enter 发送；发出去后输入框清空。
//
// draft 是本帧从界面状态复制出的草稿；gen 是这条对话已发送的次数，每发一次就变：
// 输入框按 gen 重建，靠这个把上一句清掉（控件只在用户打字时回写，外部改 draft 清不掉它）。
// 返回值：这一帧要发送的文字，没有就是空串。调用方负责把 draft 写回状态。
func chatComposer(c *ui.Context, gen int, draft *string, p composerProps) string {
	tk := theme.TokensOf(c)
	before := *draft
	var send string

	chatColumn(c, func() {
		card := ui.Column(c).Grow(1).Basis(0).MaxWidth(chatWidth).Padding(14, 16, 10, 16).Gap(6).
			Radius(18).Background(tk.BgContainer).FontSize(15).TextColor(tk.Text).Cursor(ui.CursorText)
		var area ui.Element
		card.Children(func() {
			c.Key(gen)
			area = ui.TextAreaBase(c, draft).Lines(1, 8).FocusRing(false).AutoFocus().
				Placeholder(p.Placeholder).PlaceholderColor(tk.TextPlaceholder)
			// 多行输入框里 Enter 会插入换行：在末尾新敲出的换行当作「发送」。
			if *draft != before && strings.HasSuffix(*draft, "\n") && !strings.HasSuffix(before, "\n") {
				text := strings.TrimSpace(*draft)
				*draft = before
				if text != "" && !p.Busy {
					send = text
				}
			}
			text := strings.TrimSpace(*draft)
			canSend := text != "" && !p.Busy
			ui.Row(c).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				hint := "Enter 发送"
				if p.Busy && p.BusyText != "" {
					hint = p.BusyText
				}
				ui.Text(c, hint).FontSize(12).TextColor(tk.TextTertiary)
				stop := p.Busy && p.OnStop != nil
				btn := ui.ButtonBase(c).Width(32).Height(32).Radius(16).AlignItems(ui.Center).Justify(ui.Center).
					Disabled(!canSend && !stop)
				// 点击要在这一帧里读出来：OnClick 的回调在本函数返回之后才跑，改局部变量 send 就丢了。
				if btn.Clicked() {
					if stop {
						p.OnStop()
					} else if canSend {
						send = text
					}
				}
				if stop {
					btn.Background(tk.Primary).Cursor(ui.CursorPointer).Children(func() {
						ui.Icon(c, lucide.Square()).Width(12).Height(12).TextColor(tk.BgContainer)
					})
					return
				}
				if canSend {
					btn.Background(tk.Primary).Cursor(ui.CursorPointer)
				} else {
					btn.Background(tk.FillSecondary)
				}
				btn.Children(func() {
					icon := ui.Icon(c, lucide.ArrowUp()).Width(16).Height(16)
					if canSend {
						icon.TextColor(tk.BgContainer)
					} else {
						icon.TextColor(tk.TextQuaternary)
					}
				})
			})
		})
		if area.Focused() {
			card.Border(1, tk.TextQuaternary) // 聚焦时边框加深一档，不用主色：主色是近黑，整圈描黑太重
		} else {
			card.Border(1, tk.Border)
		}
		tk.Elevate(card, theme.BoxShadowSecondary)
	})
	// 底部留白：输入框和窗口边缘之间
	ui.Column(c).Shrink(0).Height(16)
	return send
}

// chatColumn 把内容放进居中的一列，宽度和输入框一致。
func chatColumn(c *ui.Context, fn func()) {
	ui.Row(c).Shrink(0).Justify(ui.Center).Padding(0, 24).Children(fn)
}
