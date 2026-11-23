import { useAssistantDataUI, useAuiState, type DataMessagePartProps } from "@assistant-ui/react"
import { BanIcon, CheckIcon, HandIcon, MessagesSquareIcon, WrenchIcon, XIcon } from "lucide-react"
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react"

import { ChatPane } from "@/components/chat/chat-pane"
import { ChatProvider } from "@/components/chat/chat-runtime"
import { Callout, ModeCallouts, ScrollBody, useSettings } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { api, chatEndpoints, type PlayIntro, type PlayStep, type ProductDetail } from "@/lib/api"
import { productHref } from "@/lib/route"

// 试玩（blueprint/playground.py）：用户扮演自己产品的使用者，看按现在的方案这个 Agent 大概会怎么表现。
// 只是演示，不是产品：不生成代码、不导出。模拟出来的动作用数据卡片显示，回复下方可以记「这里不对」，
// 最后一起带去「加新需求」。

type Note = { id: string; said: string; reply: string; comment: string }

type NotesApi = { notes: Note[]; add: (n: Note) => void; remove: (id: string) => void; clear: () => void }

const NotesContext = createContext<NotesApi | null>(null)

function useNotes() {
  const ctx = useContext(NotesContext)
  if (!ctx) throw new Error("NotesContext missing")
  return ctx
}

export const reviseSeedKey = (slug: string) => `ab:revise-seed:${slug}`

// 「这里不对」存在引擎里，和桌面版共用（blueprint/web/conversations.py 的 notes）。
function saveNotes(slug: string, notes: Note[]) {
  api.saveNotes(slug, notes).catch(() => {
    /* 存不上就只在这次打开的页面里有效 */
  })
}

/** 记下的「这里不对」整理成一段新需求的开头，放进「加新需求」的输入框，用户再补充或直接发送。 */
function seedText(notes: Note[]) {
  const lines = notes.map((n, i) => `${i + 1}. 我说「${n.said}」，它回「${n.reply}」。我希望：${n.comment}`)
  return `试玩时发现这些地方和我想的不一样：\n${lines.join("\n")}`
}

// ---------------------------------------------------------------------------
// 数据卡片：模拟的动作、「这里不对」
// ---------------------------------------------------------------------------

/** 一次模拟动作，和桌面版一样三种卡片：调用工具、要先问人、按规则拦下。 */
function StepCard({ data }: DataMessagePartProps<PlayStep>) {
  if (data.kind === "tool") {
    return (
      <div className="my-2 flex flex-col gap-1 rounded-xl border px-3.5 py-2.5 text-[13px]">
        <div className="flex items-center gap-2">
          <WrenchIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="text-text-secondary">模拟调用工具</span>
          <span className="font-mono font-semibold">{data.tool}</span>
        </div>
        {data.input && <div className="text-text-secondary">输入：{data.input}</div>}
        {data.result && <div className="text-text-secondary">结果（编的）：{data.result}</div>}
      </div>
    )
  }
  if (data.kind === "ask_owner") {
    return (
      <div className="my-2 flex items-start gap-2 rounded-xl bg-[#fff8f3] px-3.5 py-2.5 text-[13px] text-warn">
        <HandIcon className="mt-0.5 size-3.5 shrink-0" />
        <span>要先问{data.who || "人"}：{data.action}</span>
      </div>
    )
  }
  return (
    <div className="my-2 flex items-start gap-2 rounded-xl bg-[#fef8f7] px-3.5 py-2.5 text-[13px] text-bad">
      <BanIcon className="mt-0.5 size-3.5 shrink-0" />
      <span>按规则拦下：{data.rule}</span>
    </div>
  )
}

function FeedbackButton() {
  const { notes, add, remove } = useNotes()
  const id = useAuiState((s) => s.message.id)
  const reply = useAuiState((s) =>
    s.message.content.flatMap((p) => (p.type === "text" ? [p.text] : [])).join("\n"),
  )
  const said = useAuiState((s) => {
    const prev = s.thread.messages[s.message.index - 1]
    return prev ? prev.content.flatMap((p) => (p.type === "text" ? [p.text] : [])).join("\n") : ""
  })
  const running = useAuiState((s) => s.message.status?.type === "running")
  const [open, setOpen] = useState(false)
  const [comment, setComment] = useState("")
  const saved = notes.find((n) => n.id === id)

  if (running) return null
  if (saved) {
    return (
      <div className="mt-1 flex items-center gap-1.5 text-[13px] text-text-secondary">
        <CheckIcon className="size-3.5 shrink-0 text-[#00864e]" />
        <span className="min-w-0 flex-1">已记下：{saved.comment}</span>
        <Button variant="ghost" size="icon-sm" onClick={() => remove(id)} aria-label="删掉这条" title="删掉这条">
          <XIcon />
        </Button>
      </div>
    )
  }
  if (!open) {
    return (
      <Button variant="ghost" size="sm" className="mt-1" onClick={() => setOpen(true)}>
        这里不对
      </Button>
    )
  }
  return (
    <div className="mt-2 flex flex-col gap-2">
      <Textarea
        autoFocus
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        placeholder="你希望它怎么做？比如「打折的事要直接说做不了，不用转给我」"
        className="min-h-16 text-sm"
      />
      <div className="flex gap-2">
        <Button
          size="sm"
          disabled={!comment.trim()}
          onClick={() => add({ id, said, reply, comment: comment.trim() })}
        >
          记下
        </Button>
        <Button size="sm" variant="outline" onClick={() => setOpen(false)}>
          取消
        </Button>
      </div>
    </div>
  )
}

function PlayRenderers() {
  useAssistantDataUI({ name: "play_step", render: StepCard })
  useAssistantDataUI({ name: "play_feedback", render: FeedbackButton })
  return null
}

// ---------------------------------------------------------------------------

function NotesBar({ slug }: { slug: string }) {
  const { notes, clear } = useNotes()
  if (!notes.length) return null
  const carry = () => {
    try {
      sessionStorage.setItem(reviseSeedKey(slug), seedText(notes))
    } catch {
      /* 带不过去就只是输入框是空的 */
    }
    clear() // 已经放进「加新需求」的输入框了，留着会被带第二次
    location.hash = productHref(slug, "revise")
  }
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b bg-[#fff8f3] px-5 py-2 text-[13px] text-warn">
      <span>试玩中记下了 {notes.length} 处不对</span>
      <Button variant="outline" size="sm" onClick={carry}>
        带去加新需求 →
      </Button>
    </div>
  )
}

const NO_TOOLS = {}

export function PlayTab({ product }: { product: ProductDetail }) {
  const { settings } = useSettings()
  const slug = product.slug
  const [intro, setIntro] = useState<PlayIntro | null>(null)
  const [error, setError] = useState("")
  const [notes, setNotes] = useState<Note[]>([])
  useEffect(() => {
    let live = true
    api.conversations(slug).then(
      (c) => live && setNotes(c.notes),
      () => {},
    )
    return () => {
      live = false
    }
  }, [slug])

  const update = useCallback(
    (next: Note[]) => {
      setNotes(next)
      saveNotes(slug, next)
    },
    [slug],
  )
  const notesApi: NotesApi = {
    clear: () => update([]),
    notes,
    add: (n: Note) => update([...notes.filter((x) => x.id !== n.id), n]),
    remove: (id: string) => update(notes.filter((x) => x.id !== id)),
  }

  useEffect(() => {
    if (!settings.configured) return
    let live = true
    api.playIntro(slug).then(
      (r) => live && setIntro(r),
      (e: Error) => live && setError(e.message),
    )
    return () => {
      live = false
    }
  }, [slug, product.version, product.updated, settings.configured])

  const initialMessages = useMemo(() => (intro ? [{ role: "assistant" as const, content: intro.text }] : []), [intro])

  if (!settings.configured) return <ScrollBody><ModeCallouts /></ScrollBody>
  if (error) {
    return (
      <ScrollBody>
        <Callout
          tone="bad"
          title="试玩没能开始"
          action={error.includes("模型设置") ? <Button size="sm" nativeButton={false} render={<a href="#/settings" />}>去模型设置</Button> : undefined}
        >
          {error}
        </Callout>
      </ScrollBody>
    )
  }
  if (!intro) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
        <MessagesSquareIcon className="mb-1.5 size-8 text-text-quaternary" />
        <div className="text-[17px] font-semibold">正在准备试玩…</div>
        <p className="max-w-[400px] text-muted-foreground">Agent 会先说明它扮演谁、能帮你做什么，然后你以使用者的身份跟它聊。</p>
      </div>
    )
  }

  return (
    <NotesContext.Provider value={notesApi}>
      <ChatProvider
        // 方案变了（提示词、工具、规则），开场白的指纹就变了，换一段新的；旧的引擎也不再返回
        key={intro.fingerprint}
        endpoint={chatEndpoints.play(slug)}
        toolkit={NO_TOOLS}
        persist={{ slug, kind: "play" }}
        initialMessages={initialMessages}
        initialChips={intro.scenarios}
      >
        <PlayRenderers />
        <ChatPane
          hint="这是按方案模拟的，不是做出来的产品：工具结果都是编的，真正的产品要开发者按交接包来做。"
          top={<NotesBar slug={slug} />}
          placeholder={`以「${intro.role}」的身份说点什么…`}
        />
      </ChatProvider>
    </NotesContext.Provider>
  )
}
