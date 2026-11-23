import type { Toolkit, ToolCallMessagePartProps } from "@assistant-ui/react"
import { useState } from "react"

import { useCardEdit } from "@/components/chat/chat-storage"
import { JobProgress, useJob } from "@/components/job-progress"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api, type Job } from "@/lib/api"

// 服务端（blueprint/chat.py）发来 tool 事件时，对话里弹出这些卡片。卡片是 human tool：
// 用户在卡片上确认后调用 addResult，对话随之结束；之后卡片原地变成推导进度。

// 和桌面版的卡片一样：细边框、12 圆角，标题 14 加粗
const CARD = "my-1 flex flex-col gap-2.5 rounded-xl border bg-card px-[18px] py-4"
const CARD_TITLE = "text-sm font-semibold"

type StartedResult = { slug: string; job: Job }

// ---------------------------------------------------------------------------
// 新建产品：确认需求草稿
// ---------------------------------------------------------------------------

type ProductArgs = { name: string; idea: string }

function ConfirmProductCard({ args, result, status, addResult }: ToolCallMessagePartProps<ProductArgs, StartedResult>) {
  const [name, setName] = useState(args.name ?? "")
  const [idea, setIdea] = useState(args.idea ?? "")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  if (result) return <StartedCard result={result} title={`「${name}」`} />

  const waiting = status.type === "requires-action"
  const submit = async () => {
    if (!name.trim() || !idea.trim()) return setError("产品名和需求都要有内容。")
    setBusy(true)
    setError("")
    try {
      const created = await api.createProduct(name, idea)
      addResult(created)
      // 和桌面版一样：建好就去产品页，推导进度在概览上看
      location.hash = `#/p/${encodeURIComponent(created.slug)}`
    } catch (err) {
      setError((err as Error).message)
      setBusy(false)
    }
  }

  return (
    <div className={CARD}>
      <div className={CARD_TITLE}>需求草稿</div>
      <label className="text-[13px] font-medium" htmlFor="draft-name">产品名</label>
      <Input id="draft-name" value={name} onChange={(e) => setName(e.target.value)} disabled={!waiting || busy} maxLength={40} />
      <label className="text-[13px] font-medium" htmlFor="draft-idea">需求</label>
      <Textarea
        id="draft-idea"
        value={idea}
        onChange={(e) => setIdea(e.target.value)}
        disabled={!waiting || busy}
        className="min-h-56 font-normal leading-relaxed"
      />
      <p className="text-xs text-muted-foreground">
        「还没想清楚」里的问题不用现在回答，方案生成后会列成待确认问题，到时候再一题一题过。
      </p>
      {error && <p className="text-[13px] text-bad">{error}</p>}
      {waiting && (
        <Button className="self-start" onClick={submit} disabled={busy}>
          {busy ? "正在创建…" : "开始推导方案"}
        </Button>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// 回答待确认问题：确认回答汇总
// ---------------------------------------------------------------------------

type AnswersArgs = { answers: { question: string; answer: string }[] }

function makeConfirmAnswersCard(slug: string, onStarted: () => void) {
  return function ConfirmAnswersCard({ args, result, status, addResult }: ToolCallMessagePartProps<AnswersArgs, StartedResult>) {
    const edit = useCardEdit()
    const [answers, setAnswersState] = useState(edit.initial?.answers ?? args.answers ?? [])
    const setAnswers = (next: typeof answers) => {
      setAnswersState(next)
      edit.set({ answers: next })
    }
    const [busy, setBusy] = useState(false)
    const [error, setError] = useState("")

    if (result) return <StartedCard result={result} title="回答已写进需求文档" />

    const waiting = status.type === "requires-action"
    const submit = async () => {
      setBusy(true)
      setError("")
      try {
        const res = await api.saveAnswers(slug, answers)
        addResult({ slug, job: res.job })
        onStarted() // 产品页顶部接手显示进度，推导完自动换成新方案
      } catch (err) {
        setError((err as Error).message)
        setBusy(false)
      }
    }

    return (
      <div className={CARD}>
        <div className={CARD_TITLE}>回答汇总 · {answers.length} 条</div>
        {answers.map((a, i) => (
          <div key={i} className="flex flex-col gap-1.5">
            <div className="flex gap-2">
              <span className="font-mono text-xs leading-5 text-muted-foreground">{String(i + 1).padStart(2, "0")}</span>
              <span className="text-[13px] text-text-secondary">{a.question}</span>
            </div>
            <Textarea
              value={a.answer}
              disabled={!waiting || busy}
              onChange={(e) => setAnswers(answers.map((x, j) => (j === i ? { ...x, answer: e.target.value } : x)))}
              className="min-h-9"
            />
          </div>
        ))}
        {error && <p className="text-[13px] text-bad">{error}</p>}
        {waiting && (
          <Button className="self-start" onClick={submit} disabled={busy}>
            {busy ? "正在保存…" : "保存并重新推导方案"}
          </Button>
        )}
      </div>
    )
  }
}

// ---------------------------------------------------------------------------
// 加新需求：确认需求变更草稿
// ---------------------------------------------------------------------------

type RevisionArgs = { title: string; requirement: string }

function makeConfirmRevisionCard(slug: string, nextVersion: number, onStarted: () => void) {
  return function ConfirmRevisionCard({ args, result, status, addResult }: ToolCallMessagePartProps<RevisionArgs, StartedResult>) {
    const edit = useCardEdit()
    const [title, setTitleState] = useState(edit.initial?.title ?? args.title ?? "")
    const [requirement, setRequirementState] = useState(edit.initial?.requirement ?? args.requirement ?? "")
    const setTitle = (t: string) => {
      setTitleState(t)
      edit.set({ title: t, requirement })
    }
    const setRequirement = (r: string) => {
      setRequirementState(r)
      edit.set({ title, requirement: r })
    }
    const [busy, setBusy] = useState(false)
    const [error, setError] = useState("")

    if (result) return <StartedCard result={result} title={`已升到 V${nextVersion}：${title}`} />

    const waiting = status.type === "requires-action"
    const submit = async () => {
      if (!requirement.trim()) return setError("新需求不能为空。")
      setBusy(true)
      setError("")
      try {
        const res = await api.startRevise(slug, title, requirement)
        addResult({ slug, job: res.job })
        onStarted() // 产品页顶部接手显示进度，推导完跳到「版本历史」看选型变化
      } catch (err) {
        setError((err as Error).message)
        setBusy(false)
      }
    }

    return (
      <div className={CARD}>
        <div className={CARD_TITLE}>需求变更草稿 · V{nextVersion}</div>
        <label className="text-[13px] font-medium" htmlFor="rev-title">一句话标题</label>
        <Input id="rev-title" value={title} onChange={(e) => setTitle(e.target.value)} disabled={!waiting || busy} maxLength={40} />
        <label className="text-[13px] font-medium" htmlFor="rev-body">新需求</label>
        <Textarea
          id="rev-body"
          value={requirement}
          onChange={(e) => setRequirement(e.target.value)}
          disabled={!waiting || busy}
          className="min-h-48 leading-relaxed"
        />
        <p className="text-xs text-muted-foreground">
          确认后，需求文档升到 V{nextVersion}（旧版本会留一份），方案按新需求重新推导；没变的部分沿用上次的结果，不重复花钱。
        </p>
        {error && <p className="text-[13px] text-bad">{error}</p>}
        {waiting && (
          <Button className="self-start" onClick={submit} disabled={busy}>
            {busy ? "正在升级…" : `升到 V${nextVersion} 并重新推导`}
          </Button>
        )}
      </div>
    )
  }
}

// ---------------------------------------------------------------------------

function StartedCard({ result, title }: { result: StartedResult; title: string }) {
  const job = useJob(result.job)
  if (!job) return null
  return (
    <div className={CARD}>
      <div className={CARD_TITLE}>{title}</div>
      <JobProgress job={job} compact />
      {job.status === "failed" && <p className="text-xs text-muted-foreground">到概览里点「继续推导」可以从断点接着跑。</p>}
      <Button
        className="self-start"
        variant="outline"
        size="sm"
        nativeButton={false}
        render={<a href={`#/p/${encodeURIComponent(result.slug)}`} />}
      >
        {job.status === "done" ? "查看方案" : "打开概览"}
      </Button>
    </div>
  )
}

const ANY_OBJECT = { type: "object" } as const

export const intakeToolkit: Toolkit = {
  confirm_product: { type: "human", parameters: ANY_OBJECT, render: ConfirmProductCard },
}

export function reviseToolkit(slug: string, nextVersion: number, onStarted: () => void): Toolkit {
  return {
    confirm_revision: { type: "human", parameters: ANY_OBJECT, render: makeConfirmRevisionCard(slug, nextVersion, onStarted) },
  }
}

export function answersToolkit(slug: string, onStarted: () => void): Toolkit {
  return {
    confirm_answers: { type: "human", parameters: ANY_OBJECT, render: makeConfirmAnswersCard(slug, onStarted) },
  }
}
