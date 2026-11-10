// Agent 工坊 桌面界面（第一阶段骨架）：用 Lobe MyGo 原生 UI 重画的网页版前端。
//
// 架构：Go 原生界面（本目录）+ Python 引擎（blueprint/）当后台大脑。
// 启动时拉起 `python -m blueprint ui --no-browser --port 0`，从 stdout 解析
// 「界面已启动：http://127.0.0.1:PORT/」拿到端口，之后走现有 /api/* JSON 接口。
// 引擎一行不改，网页版（web/）原样保留。
package main

import (
	"flag"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"

	// 注册应用文案目录，让 MyGo 与 Lobe 组件的内建文案讲中文。
	_ "github.com/hubooooooo/agent-workshop/desktop/locales"
)

// -mock：用引擎的离线启发式模式演示（不调模型、不花钱）
var mockMode = flag.Bool("mock", false, "引擎离线演示模式（不调用模型）")

// invalidate 由每帧的 root 视图赋值；后台 goroutine 取到数据后调它触发重绘。
var invalidate func()

var app = newState()

func root(c *ui.Context) {
	invalidate = c.Invalidate
	theme.Apply(c)
	tk := theme.TokensOf(c)
	ui.Column(c).Fill().Background(tk.BgLayout).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			sidebar(c)
			content(c)
		})
	})
}

func windowOptions() mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:    "Agent 工坊",
		Width:    1280,
		Height:   820,
		MinWidth: 960,
		MinHeight: 600,
		Content:  ui.View(root),
	}
}

func main() {
	flag.Parse()
	mygo.App.WhenReady(func() {
		mygo.App.SetLocale("zh-Hans")
		mygo.NewWindow(windowOptions())
		go app.startEngine(*mockMode)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	app.stopEngine()
}
