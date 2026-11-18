# Agent 工坊插件（Claude Code）

在你的 AI 编程工具里，用聊天的方式把一个 AI Agent 点子变成：

- **`PRD.md`**：AI 能看懂的需求文档
- **`方案.md`**：技术方案（选哪些机制、分几层、有哪些工具、怎么防出错）

然后按方案进入开发。不用装 Python 环境，也不用另配模型 API Key：所有工作由你正在用的 AI 编程工具完成。

## 安装

在终端里运行（Claude Code 2.1.292 及以上）：

```bash
claude plugin install agent-workshop --marketplace hubooooooo/agent-workshop
```

或者在 Claude Code 对话框里输入：

```text
/plugin install agent-workshop --marketplace hubooooooo/agent-workshop
```

版本较旧装不上时，分两步：

```bash
claude plugin marketplace add hubooooooo/agent-workshop
claude plugin install agent-workshop@agent-workshop
```

更新到新版本：

```bash
claude plugin update agent-workshop@agent-workshop
```

## 怎么用

装好后直接说出你的想法就行，比如：

> 我想做个 AI，帮我妈记账。她在微信里发一句话就能记下来，月底给她一个总结。

AI 会一次问你一件事，问清楚后整理成草稿给你确认，确认后在项目的 `docs/{产品名}/` 下写出 `PRD.md` 和 `方案.md`。说不清的地方不用硬想，直接说「不知道」，它会列成待确认问题，你之后再回答。

之后可以：

- **回答待确认问题**：「我来回答方案里的问题」，答案会写进 PRD，方案随之更新。
- **加新需求**：「再加一个功能：……」，PRD 会升一版，方案里写明这一版改了什么。
- **进入开发**：「按方案开始开发」。

## 包含的技能

| 技能 | 做什么 |
|---|---|
| `agent-prd` | 追问点子，产出 PRD.md 和 方案.md；回答问题、加新需求 |
| `agent-builder` | 按确认过的方案开发：代码骨架、工具、权限、容错、日志、测试 |

也可以直接点名调用：`/agent-workshop:agent-prd`、`/agent-workshop:agent-builder`。

## 不用 Claude Code 也能用

`agent-prd` 是一个独立的技能文件夹，不依赖插件，打好的文件在[下载页](https://github.com/hubooooooo/agent-workshop/releases/tag/agent-prd)。claude.ai、Codex、Cursor 等其他 AI 工具，甚至普通聊天网页的用法，见 [skills/agent-prd/README.md](skills/agent-prd/README.md)。

## 只适合 Agent 类产品

这套方法为「AI 自己理解输入、调用工具、做判断、交付结果」的产品设计（客服、助理、自动整理、审核……）。普通 App、网站或固定流程的脚本，用它做出的方案会偏离需要。

## 和 Agent 工坊引擎的关系

方法来自 [Agent 工坊](https://github.com/hubooooooo/agent-workshop) 引擎：同一套追问规则、组件选型规则和文档结构。引擎另外提供桌面版、网页版和命令行，以及由另一个模型做的独立审稿；插件版由你的 AI 编程工具自己完成推导，交付前做的是作者自查。两边产出的 PRD.md / 方案.md 可以互相接着用。
