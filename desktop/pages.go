package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// pageShell 是右栏每一页的外壳：圆角卡片 + 顶部 52 高的标题栏（下方一条细分隔线）+ 正文。
// 所有页面共用，保证标题栏高度、内边距、分隔线完全一致。
func pageShell(c *ui.Context, header func(), body func()) {
	tk := theme.TokensOf(c)
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Margin(8, 8, 8, 0).
		Radius(tk.BorderRadiusLG).Background(tk.BgContainer).Border(1, tk.Border).Clip().
		Children(func() {
			ui.Row(c).Height(52).Shrink(0).Padding(0, 12, 0, 20).Gap(12).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(header)
			body()
		})
}

// pageTitle 标题栏左侧：页面名 + 可选的一句灰色说明。
func pageTitle(c *ui.Context, title, sub string) {
	tk := theme.TokensOf(c)
	ui.Row(c).Gap(10).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
		ui.Text(c, title).FontSize(15).Bold().Shrink(0)
		if sub != "" {
			ui.Text(c, sub).FontSize(13).TextColor(tk.TextTertiary).MaxLines(1)
		}
	})
}

// readingScroll 可滚动的阅读区：内容居中成一列，和对话页同宽，窗口再宽也不铺满。
func readingScroll(c *ui.Context, gap float32, fn func()) {
	ui.Scroll(c).Grow(1).Basis(0).Children(func() {
		chatColumn(c, func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).MaxWidth(chatWidth).Padding(28, 0, 40, 0).Gap(gap).Children(fn)
		})
	})
}

// sectionCard 方案、报告里的一节：细边框圆角卡片，标题旁可带数量。
func sectionCard(c *ui.Context, title string, count int, fn func()) {
	tk := theme.TokensOf(c)
	ui.Column(c).Padding(16, 18).Gap(12).Radius(12).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(14).Bold()
			if count > 0 {
				ui.Text(c, fmt.Sprint(count)).FontSize(12).TextColor(tk.TextTertiary).
					Padding(1, 7).Radius(8).Background(tk.FillTertiary)
			}
		})
		fn()
	})
}

// centerEmpty 页面正中的空状态：图标、一句标题、一句说明、可选按钮。
// 不直接用 display.Empty：它把说明限制在两百多像素宽，中文一句话会折成三四行。
func centerEmpty(c *ui.Context, p display.EmptyProps) {
	tk := theme.TokensOf(c)
	ui.Column(c).Grow(1).Basis(0).Justify(ui.Center).AlignItems(ui.Center).Padding(24).Gap(8).Children(func() {
		if p.Icon != nil {
			ui.Icon(c, p.Icon).Width(32).Height(32).TextColor(tk.TextQuaternary).Margin(0, 0, 6, 0)
		}
		if p.Title != "" {
			ui.Text(c, p.Title).FontSize(17).FontWeight(600).TextAlign(ui.Center)
		}
		if p.Description != "" {
			ui.Text(c, p.Description).FontSize(14).TextColor(tk.TextTertiary).TextAlign(ui.Center).MaxWidth(400)
		}
		if p.Action != nil {
			ui.Row(c).Gap(8).Margin(10, 0, 0, 0).Children(p.Action)
		}
	})
}

// errorAlert 统一的出错提示：浅红底 + 图标，代替裸露的「⚠ 文本」。
func errorAlert(c *ui.Context, msg string) {
	feedback.Alert(c, msg, feedback.AlertProps{Type: feedback.Error})
}

// mutedText 加载中、没内容这类次要提示。
func mutedText(c *ui.Context, text string) {
	tk := theme.TokensOf(c)
	ui.Text(c, text).FontSize(13).TextColor(tk.TextTertiary)
}

// gotoSettings 跳到模型设置页（试玩没配模型时用）。
func gotoSettings() {
	app.mu.Lock()
	app.page = PageSettings
	app.mu.Unlock()
	app.invalidate()
	go app.loadSettings()
}

// breaks 让单个换行真的换行，和网页版的 remark-breaks 一样：引擎和模型写的「A. …\nB. …」
// 是想分行显示的，标准 Markdown 却会把它们并成一段。代码块和表格行原样不动。
func breaks(src string) string {
	lines := strings.Split(src, "\n")
	fenced := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || t == "" || strings.HasPrefix(t, "|") || i == len(lines)-1 {
			continue
		}
		if strings.TrimSpace(lines[i+1]) == "" || strings.HasSuffix(l, "  ") {
			continue
		}
		lines[i] = l + "  "
	}
	return strings.Join(lines, "\n")
}
