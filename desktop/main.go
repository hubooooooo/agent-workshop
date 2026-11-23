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
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/theme"

	// 注册应用文案目录，让 MyGo 与 Lobe 组件的内建文案讲中文。
	_ "github.com/hubooooooo/agent-workshop/desktop/locales"
)

// -mock：用引擎的离线启发式模式演示（不调模型、不花钱）
var mockMode = flag.Bool("mock", false, "引擎离线演示模式（不调用模型）")

// invalidate 由每帧的 root 视图赋值；后台 goroutine 取到数据后调它触发重绘。
// 界面线程写、后台 goroutine 读，用原子指针，免得数据竞争。
var invalidate atomic.Pointer[func()]

var app = newState()

func root(c *ui.Context) {
	fn := c.Invalidate
	invalidate.Store(&fn)
	theme.Apply(c)
	tk := theme.TokensOf(c)
	ui.Column(c).Fill().Background(tk.BgLayout).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			sidebar(c)
			content(c)
		})
	})
	restartConfirm(c)
	drainToasts(c)
	feedback.ToastHost(c, feedback.ToastHostProps{})
}

// drainToasts 把后台排好的提示在界面线程弹出来（和网页版右下角的 toast 一样）。
func drainToasts(c *ui.Context) {
	app.mu.Lock()
	pending := app.toasts
	app.toasts = nil
	app.mu.Unlock()
	for _, t := range pending {
		kind := feedback.ToastError
		if t.ok {
			kind = feedback.ToastSuccess
		}
		feedback.ShowToast(c, feedback.ToastProps{Description: t.text, Type: kind})
	}
}

func windowOptions() mygo.WindowOptions {
	return mygo.WindowOptions{
		Title:     "Agent 工坊",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 600,
		Content:   ui.View(root),
	}
}

func main() {
	flag.Parse()
	// 退出时一定带走引擎：App.Run 之后的 stopEngine 在强制退出（os.Exit）时走不到，
	// 引擎就留在后台占着端口。OnWillQuit 在事件循环停下前调用；收到终止信号也先停引擎。
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { app.stopEngine() })
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigs
		app.stopEngine()
	}()
	// 只做浅色：不跟随系统的深色模式。在 Run 之前设好，窗口一出来就是浅色，
	// 标题栏、菜单这些系统画的部分也跟着是浅色，theme.Apply 也就只会取到浅色配色。
	mygo.Theme.SetSource(mygo.ThemeLight)
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
