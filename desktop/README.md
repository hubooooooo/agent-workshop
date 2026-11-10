# Agent 工坊 桌面版（Lobe MyGo 原生界面）

网页版前端的原生重画版：用 [Lobe MyGo](https://github.com/lobehub/lobe-mygo)（Go 原生 UI，
GPU 绘制，无 HTML/JS/webview）把界面做成本地桌面应用。**引擎一行不改**：应用拉起
`python -m blueprint ui --no-browser`，解析 stdout 里的端口后走现有 `/api/*` JSON 接口。
`web/` 网页版原样保留，两个界面共用同一个引擎。

## 功能

- 产品列表 + PRD Markdown 渲染
- 结构化方案页：待确认清单、组件选型、七层架构、工具清单、审稿
- 试玩：开场白 + 流式对话 + 模拟动作卡（工具名/输入/结果）
- 对话式新建产品（NDJSON 流式追问 + 确认卡片）
- 模型设置（服务商/模型/Key/接口地址，保存写回 .env）
- 外部项目检查：选文件夹 → 差距报告 + 本该有的 PRD
- 交接包下载（存到 ~/Downloads 并在 Finder 高亮）
- 推导/检查任务进度：每 2 秒增量轮询日志

## 运行（开发）

前置：仓库根有 `.venv`（跑过 `setup.sh`），Go 1.27.3+。

```bash
cd desktop
GOTOOLCHAIN=go1.27.3 GOPROXY=https://goproxy.cn,direct go run .       # 真实模式（读 .env 的 Key）
GOTOOLCHAIN=go1.27.3 GOPROXY=https://goproxy.cn,direct go run . -mock # 离线演示（不调模型）
```

> 国内直连 proxy.golang.org 会超时，必须用 `GOPROXY=https://goproxy.cn,direct`；
> 老 Go 版本靠 `GOTOOLCHAIN=go1.27.3` 自动下载新工具链。

## 打包（macOS）

```bash
cd desktop
GOTOOLCHAIN=go1.27.3 GOPROXY=https://goproxy.cn,direct go tool mygo build
```

产物在 `dist/darwin-arm64/`：`Agent 工坊.app`（12.5 MB）与
`agent-gong-fang-0.1.0-arm64.dmg`（4.1 MB）。应用名、版本、图标等配在 `mygo.json`，
图标是 `resources/icon.png`（1024×1024 最佳）。正式分发前把
`macos.signingIdentity` 换成开发者证书并配置公证。

## 结构

| 文件 | 职责 |
|---|---|
| `main.go` | 窗口、根视图、locale |
| `engine.go` | 拉起 Python 引擎、解析端口、HTTP/NDJSON 客户端、交接包下载 |
| `state.go` | 全部界面状态 + 后台数据拉取 + 任务轮询（锁保护，视图只读） |
| `view.go` | 各页面视图（Lobe MyGo 组件） |
| `locales/` | zh-Hans 文案目录（组件内建文案中文）；`catalog_gen.go` 由 `mygo generate` 生成 |

## 约束与约定

- **应用必须放在 agent-workshop 仓库里运行**：靠向上找 `blueprint/` + `.venv/`
  定位仓库根（先按应用自身路径，再按工作目录）。打包的 .app 放在仓库里任意位置都能用。
- 引擎退出原因会显示在侧栏底部并透传到终端；`--port 0` 让引擎自己挑空闲端口，
  端口从 stdout 的「界面已启动」行解析。
- 依赖 pin 到具体 commit（lobe-mygo `5b3ecde8`、mygo `v0.8.4`，见 go.mod），
  升级前先全量回归：`go build ./...` + 跑一遍上面所有功能。
