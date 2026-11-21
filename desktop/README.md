# Agent 工坊 桌面版（Mac，预览）

网页版界面的 macOS 原生重画：用 [Lobe MyGo](https://github.com/lobehub/lobe-mygo)（Go 原生 UI，GPU 绘制，没有 HTML/JS/webview）做成本地桌面应用。

**引擎一行不改**：应用启动时在后台拉起 `python -m blueprint ui --no-browser --port 0`，从 stdout 解析端口，之后走网页版同一套 `/api/*` JSON 接口。`web/` 网页版原样保留，两个界面共用同一个引擎和同一个 `out/` 目录，在哪边建的产品另一边都看得到。

## 能做什么

左栏是四个入口（产品、新建产品、项目检查、模型设置）和「我的产品」列表，底部一个小圆点显示引擎状态（启动中 / 已连接 / 出错）。

- **产品页**：标题栏中间切换 PRD / 方案 / 试玩，右边是交接状态、导出交接包、在 Finder 里打开产品文件夹。
  - PRD：Markdown 渲染，居中成一列阅读。
  - 方案：分节卡片展示待确认清单、已确认的事实、组件选型、七层架构、工具清单、降级与安全、测试设计、系统提示词草案、独立审稿。
  - 试玩：开场白 + 流式对话，每次工具调用是一张卡片（工具名、输入、结果）。还没配模型时直接提示去「模型设置」，不显示输入框。
- **新建产品**：像聊天一样说想法，AI 一次问一个问题，问清楚后出一张需求草稿卡，确认后开始推导，进度每 2 秒更新一次。
- **项目检查**：选一个已有的 Agent 项目文件夹，得到差距报告和一份「本该有的 PRD」；检查过的项目列在左边，可以重新检查。
- **模型设置**：服务商、模型、API Key、接口地址，保存后写回仓库根的 `.env`。
- **交接包**：下载到 `~/Downloads`，并在 Finder 里选中。

对话页底部是全宽居中的输入框：文字多了自动增高（最多 8 行），Enter 发送，AI 回复时可以继续打字但不能发送。

界面固定为浅色，不跟随系统的深色模式。

**还没做的**（先用网页版）：回答问题、加新需求、版本历史、项目检查的「重新判断」。

## 运行（开发）

前置：仓库根有 `.venv`（跑过 `setup.sh`），Go 1.27.3 或更新。

```bash
cd desktop
GOPROXY=https://goproxy.cn,direct go run .        # 真实模式（读 .env 的 Key）
GOPROXY=https://goproxy.cn,direct go run . -mock  # 离线演示（不调模型、不花钱）
```

> 国内直连 proxy.golang.org 会超时，要加 `GOPROXY=https://goproxy.cn,direct`。本机 Go 比 1.27.3 旧的话，再加 `GOTOOLCHAIN=go1.27.3` 让它自动下载新工具链。

测试（不需要 Key，也不启动引擎）：

```bash
cd desktop
go vet ./... && go test ./...
```

## 打包（macOS）

```bash
cd desktop
GOPROXY=https://goproxy.cn,direct go tool mygo build
```

产物在 `dist/darwin-arm64/`：`Agent 工坊.app`（约 13 MB）和 `agent-gong-fang-0.1.0-arm64.dmg`（约 4 MB）。应用名、版本、包名配在 `mygo.json`，图标是 `resources/icon.png`（1024×1024 最佳）。现在没有签名和公证，正式分发前要配开发者证书。

## 结构

| 文件 | 职责 |
|---|---|
| `main.go` | 窗口、根视图、语言、固定浅色主题；退出时停掉引擎 |
| `engine.go` | 拉起 Python 引擎、解析端口、HTTP / NDJSON 客户端、交接包下载 |
| `state.go` | 全部界面状态、后台数据拉取、任务轮询（加锁，视图只读） |
| `view.go` | 左栏和各页面 |
| `pages.go` | 各页共用的部件：页面外壳和标题栏、居中阅读列、分节卡片、空状态、出错提示 |
| `composer.go` | 对话页底部的输入框 |
| `*_test.go` | 输入框发送与清空、状态逻辑的测试 |
| `locales/` | zh-Hans 文案目录（让组件内建文案讲中文）；`catalog_gen.go` 由 `mygo generate` 生成 |
| `mygo.json` · `resources/` | 打包配置和图标 |

## 约束与约定

- **应用要放在 agent-workshop 仓库里运行**：它向上找同时有 `blueprint/` 和 `.venv/` 的目录当仓库根（先按应用自身的路径找，再按工作目录找）。打包出的 .app 放在仓库里任何位置都能用。
- 引擎出错退出时，原因显示在左栏底部，同时输出到终端。`--port 0` 让引擎自己挑空闲端口，端口从 stdout 的「界面已启动」那一行解析。
- 退出应用或收到终止信号时都会先停掉引擎，不会留在后台占端口。
- 依赖钉在具体 commit（lobe-mygo `5b3ecde8`、mygo `9233663736ca`，见 `go.mod`）。升级前先全量回归：`go vet ./... && go test ./...`，再打包把上面每个功能点一遍。
