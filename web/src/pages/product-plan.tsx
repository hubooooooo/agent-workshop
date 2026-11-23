import { CopyIcon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"

import { Card, Chip, DECISIONS } from "@/components/common"
import { Markdown } from "@/components/markdown"
import { Bullets } from "@/components/rich-text"
import { Button } from "@/components/ui/button"
import { handoffUrl, type Plan, type Question } from "@/lib/api"
import { cn } from "@/lib/utils"

// 方案的五个标签页，数据来自 blueprint/web/views.py 的 plan_view。
// 版式规则见 docs/维护手册.md「阅读版式」：正文用 PROSE；模型给的列表按列表显示，不拼成一段。

function Section({ title, aside, children, className }: { title: ReactNode; aside?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cn("mb-7 flex flex-col gap-3 last:mb-0", className)}>
      <div className="flex items-center justify-between gap-2">
        <h2 className="flex min-w-0 items-center gap-2 text-base font-semibold">{title}</h2>
        {aside}
      </div>
      {children}
    </section>
  )
}

function ReviewChip({ review }: { review: Plan["review"] }) {
  const score = review.score != null ? ` · ${review.score} 分` : ""
  return review.ok ? <Chip tone="ok">审稿通过{score}</Chip> : <Chip tone="warn">审稿未通过{score}</Chip>
}

/** 细线表格：灰色小字表头，行间一条细线（和桌面版的 tableHeader + divided 一样）。 */
function Ledger({ head, rows }: { head: string[]; rows: ReactNode[][] }) {
  return (
    <div className="-mx-4 overflow-x-auto sm:mx-0">
      <table className="w-full min-w-[36rem] border-collapse text-sm">
        <thead>
          <tr className="border-b">
            {head.map((h) => (
              <th key={h} className="px-1.5 pb-1.5 text-left text-xs font-normal text-muted-foreground first:ps-4 sm:first:ps-0">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-b last:border-b-0">
              {r.map((c, j) => (
                <td key={j} className="px-1.5 py-2.5 align-top wrap-break-word first:ps-4 sm:first:ps-0">{c}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** 组件类问题末尾的「（关系到组件 s07 技能加载）」拆出来，做成小标签。 */
function QuestionText({ q }: { q: Question }) {
  const m = q.text.match(/^(.*)（关系到组件 (.+)）$/)
  return (
    <span title={q.original === q.text ? undefined : `原问题：${q.original}`}>
      {m ? m[1] : q.text}
      {m && <span className="ms-1.5 inline-block font-mono text-xs whitespace-nowrap text-muted-foreground">{m[2]}</span>}
    </span>
  )
}

/** 概览右栏：组件选型的数量，点进去看详情。 */
function ComponentSummary({ plan, onOpen }: { plan: Plan; onOpen: () => void }) {
  return (
    <Card>
      <div className="flex items-center justify-between">
        <span className="text-[13px] text-muted-foreground">组件选型</span>
        <Button variant="ghost" size="sm" onClick={onOpen}>看详情 →</Button>
      </div>
      {DECISIONS.map((d) => (
        <div key={d.key} className="flex items-center justify-between" title={d.hint}>
          <Chip tone={d.tone}>{d.key}</Chip>
          <span className="font-mono">{plan.components.filter((c) => c.decision === d.key).length}</span>
        </div>
      ))}
    </Card>
  )
}

/** 概览右栏：下一步，导出交接包给开发者。 */
function Handoff({ plan, slug }: { plan: Plan; slug: string }) {
  const left = plan.questions.length
  return (
    <Card>
      <span className="text-xs text-muted-foreground">下一步：交给会写代码的人</span>
      <p className="leading-[1.6]">方案定下来以后，导出一个交接包（zip 文件），发给帮你做产品的开发者。</p>
      <ul className="space-y-0.5 text-xs text-muted-foreground">
        <li>交接说明.md · 从这里开始</li>
        <li>给AI编程工具的提示词.md</li>
        <li>PRD.md · 方案.md · 迭代记录.md</li>
        <li>参考组件/ · {plan.components.filter((c) => c.decision === "必选").length} 个必选组件</li>
      </ul>
      {left > 0 && (
        <p className="rounded-lg bg-[#f8f9ff] px-2.5 py-2 text-xs leading-relaxed text-link">
          还有 {left} 个问题没回答。建议先答完再导出；也可以先导出，开发者会把这些问题带回来问你。
        </p>
      )}
      <Button className="w-full" nativeButton={false} render={<a href={handoffUrl(slug)} download />}>
        导出交接包
      </Button>
      <p className="text-xs leading-relaxed text-muted-foreground">
        对方用 AI 编程工具的话，把「给AI编程工具的提示词.md」交给它就行。方案每更新一次，记得重新导出。
      </p>
    </Card>
  )
}

const SHOW_FIRST = 8

/** 关键问题（priority 1）排在最前、带标签；超过 8 个时其余的折起来，免得一眼看过去吓人。 */
function QuestionList({ questions }: { questions: Question[] }) {
  const [all, setAll] = useState(false)
  const shown = all ? questions : questions.slice(0, SHOW_FIRST)
  const key = questions.filter((q) => q.priority === 1).length
  return (
    <div className="flex flex-col gap-3">
      {key > 0 && (
        <p className="text-[13px] text-muted-foreground">
          其中 {key} 个关键问题排在最前面：不回答就没法动手，或者会让方案大改。
        </p>
      )}
      <ol>
        {shown.map((q, i) => (
          <li key={i} className="flex gap-2.5 border-t py-2.5 first:border-t-0">
            <span className="w-[22px] shrink-0 pt-0.5 font-mono text-xs text-muted-foreground">{String(i + 1).padStart(2, "0")}</span>
            <span className="min-w-0 flex-1">
              {q.priority === 1 && <Chip className="me-2 align-[1px]">关键</Chip>}
              <QuestionText q={q} />
            </span>
          </li>
        ))}
      </ol>
      {questions.length > SHOW_FIRST && (
        <Button variant="ghost" size="sm" className="self-start" onClick={() => setAll(!all)}>
          {all ? "收起" : `还有 ${questions.length - SHOW_FIRST} 个问题，展开`}
        </Button>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------

export function OverviewTab({
  plan,
  slug,
  onAnswer,
  onComponents,
}: {
  plan: Plan
  slug: string
  onAnswer: () => void
  onComponents: () => void
}) {
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_272px]">
      <div className="min-w-0">
        <Section title={<span className="flex items-center gap-2">产品定位 <ReviewChip review={plan.review} /></span>}>
          <p className="text-base leading-[1.7]">{plan.one_liner || plan.title}</p>
        </Section>

        <Section
          title={`待确认问题（${plan.questions.length}）`}
          aside={plan.questions.length ? <Button size="sm" onClick={onAnswer}>开始回答 →</Button> : null}
        >
          {plan.questions.length ? (
            <>
              <p className="text-[13px] text-muted-foreground">
                答不上来的就是风险点。点「开始回答」在对话里一题一题过，拿不准的可以让 AI 给你几个方向；答完自动重新推导。
              </p>
              <QuestionList questions={plan.questions} />
            </>
          ) : (
            <p className="text-muted-foreground">没有需要你回答的问题了。</p>
          )}
          {plan.dev_items.length > 0 && (
            <details className="rounded-lg border px-4 py-2.5">
              <summary className="cursor-pointer text-sm">
                交给开发者的技术备注 <span className="font-mono text-xs text-muted-foreground">{plan.dev_items.length}</span>
                <span className="ms-2 text-muted-foreground">· 你不用回答，交接包里会带给开发者</span>
              </summary>
              <ol className="mt-3 space-y-2 text-[13px] text-text-secondary">
                {plan.dev_items.map((q, i) => (
                  <li key={i} className="flex gap-3">
                    <span className="font-mono text-xs">{String(i + 1).padStart(2, "0")}</span>
                    <span title={q.original}>{q.text}</span>
                  </li>
                ))}
              </ol>
            </details>
          )}
        </Section>

        <Section title="需求拆解">
          <Ledger
            head={["维度", "结论", "状态"]}
            rows={plan.facts.map((f) => [
              <span className="font-medium whitespace-nowrap">{f.label}</span>,
              f.value || <span className="text-muted-foreground">（空）</span>,
              <Chip tone={f.status === "明确" ? "ok" : "ask"}>{f.status}</Chip>,
            ])}
          />
        </Section>
      </div>

      {/* 吸顶位置等于内容区上内边距（ScrollBody 的 pt-6），滚动时卡片一点不动 */}
      <aside className="flex flex-col gap-4 lg:sticky lg:top-6 lg:self-start">
        <ComponentSummary plan={plan} onOpen={onComponents} />
        <Handoff plan={plan} slug={slug} />
      </aside>
    </div>
  )
}

export function ComponentsTab({ plan }: { plan: Plan }) {
  return (
    <div>
      <p className="mb-7 text-[13px] text-muted-foreground">
        引擎内置 17 个 Harness 组件。选不选由固定规则决定：模型只判断需求有没有某个特征，同样的判断永远得到同样的选型。原则是只选需要的。
      </p>
      {DECISIONS.map((d) => {
        const items = plan.components.filter((c) => c.decision === d.key)
        if (!items.length) return null
        return (
          <Section
            key={d.key}
            title={<><Chip tone={d.tone}>{d.key}</Chip><span className="font-mono text-[13px] font-normal text-muted-foreground">{items.length}</span></>}
            aside={<span className="text-xs text-muted-foreground">{d.hint}</span>}
          >
            <Rows>
              {items.map((c) => (
                <div key={c.id} className="flex flex-wrap gap-x-3 gap-y-1 sm:flex-nowrap">
                  <span className="w-[34px] shrink-0 pt-0.5 font-mono text-xs text-muted-foreground">{c.id}</span>
                  <div className="flex w-[150px] shrink-0 flex-col gap-0.5">
                    <span className={cn("font-semibold", d.tone === "muted" && "text-muted-foreground")}>{c.name}</span>
                    <span className="text-xs text-muted-foreground">{c.domain}</span>
                  </div>
                  <p className="min-w-0 flex-1 basis-full text-[13px] text-text-secondary wrap-anywhere sm:basis-0">{c.reason}</p>
                </div>
              ))}
            </Rows>
          </Section>
        )
      })}
    </div>
  )
}

const PERMISSION = { allow: ["直接执行", "ok"], ask: ["需人确认", "warn"], deny: ["禁止", "bad"] } as const

export function ArchitectureTab({ plan }: { plan: Plan }) {
  return (
    <div>
      <Section title="七层架构">
        <Rows>
          {plan.layers.map((l) => (
            <div
              key={l.num}
              className={cn("flex flex-col gap-x-3.5 gap-y-1 rounded-lg px-2 py-1 sm:flex-row", l.num === "③" && "bg-black/[0.015]")}
            >
              <div className="flex w-[220px] shrink-0 flex-col gap-0.5">
                <span className="font-semibold">{l.num} {l.name}</span>
                {l.question && <span className="text-xs text-muted-foreground">{l.question}</span>}
              </div>
              <p className="min-w-0 flex-1 text-[13px] text-text-secondary wrap-anywhere">{l.detail}</p>
            </div>
          ))}
        </Rows>
      </Section>

      <Section title="工具清单" aside={<span className="font-mono text-[13px] text-muted-foreground">{plan.tool_list.length}</span>}>
        {plan.tool_list.length ? (
          plan.tool_list.map((t) => {
            const [label, tone] = PERMISSION[t.permission] ?? [t.permission, "muted"]
            return (
              <Card key={t.name}>
                <div className="flex flex-wrap items-center gap-2">
                  <code className="font-mono text-[13px] font-semibold wrap-anywhere">{t.name}</code>
                  <Chip tone={tone}>{label}</Chip>
                  {t.layer && <span className="text-xs text-muted-foreground">第 {t.layer} 层</span>}
                </div>
                {([["输入", t.input], ["输出", t.output], ["副作用", t.side_effect]] as const).map(([k, v]) => (
                  <div key={k} className="flex gap-2.5">
                    <span className="w-11 shrink-0 text-xs leading-5 text-muted-foreground">{k}</span>
                    <span className="min-w-0 flex-1 text-[13px] text-text-secondary wrap-anywhere">{v || "—"}</span>
                  </div>
                ))}
              </Card>
            )
          })
        ) : (
          <p className="text-[13px] text-muted-foreground">（没有生成工具清单）</p>
        )}
      </Section>

      <Section title="五要素">
        <Rows>
          {plan.elements.map((e) => (
            <div key={e.label} className="flex flex-col gap-x-3 gap-y-1 sm:flex-row">
              <span className="w-[88px] shrink-0 font-semibold">{e.label}</span>
              <div className="min-w-0 flex-1">
                {e.items.length ? <Bullets items={e.items} /> : <span className="text-[13px] text-muted-foreground">（待补充）</span>}
              </div>
            </div>
          ))}
        </Rows>
      </Section>
    </div>
  )
}

/** 一组行，行与行之间一条细线（和桌面版的 divided 一样）。 */
function Rows({ children }: { children: ReactNode }) {
  return <div className="flex flex-col divide-y [&>*]:py-2.5">{children}</div>
}

function BulletsOrEmpty({ items }: { items?: string[] }) {
  return items?.length ? <Bullets items={items} /> : <span className="text-[13px] text-muted-foreground">（空）</span>
}

function MarkdownOrEmpty({ text }: { text?: string }) {
  return text?.trim() ? <Markdown text={text} /> : <span className="text-[13px] text-muted-foreground">（空）</span>
}

export function PromptTab({ plan }: { plan: Plan }) {
  const obs = plan.observability
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(plan.system_prompt)
      toast.success("已复制系统提示词")
    } catch {
      toast.error("复制失败，请手动选中复制")
    }
  }
  return (
    <div>
      <Section
        title="系统提示词草案"
        aside={plan.system_prompt ? <Button variant="ghost" size="icon-sm" onClick={copy} title="复制" aria-label="复制"><CopyIcon /></Button> : null}
      >
        <pre className="rounded-lg bg-black/[0.015] px-3.5 py-3 font-mono text-[13px] leading-[1.6] whitespace-pre-wrap text-text-secondary">
          {plan.system_prompt || "（空）"}
        </pre>
      </Section>
      <Section title="上下文与记忆策略">
        <MarkdownOrEmpty text={plan.context_memory_strategy} />
      </Section>
      <Section title="安全边界与降级策略">
        <BulletsOrEmpty items={plan.security_and_degradation} />
      </Section>
      <Section title="可观测性">
        {([
          ["日志", <BulletsOrEmpty items={obs.logs} />],
          ["指标", <BulletsOrEmpty items={obs.metrics} />],
          ["轨迹", <MarkdownOrEmpty text={obs.trajectory} />],
        ] as const).map(([label, body]) => (
          <div key={label} className="flex gap-3">
            <span className="w-11 shrink-0 font-semibold">{label}</span>
            <div className="min-w-0 flex-1">{body}</div>
          </div>
        ))}
      </Section>
      <Section title="技术栈与部署形态">
        <MarkdownOrEmpty text={plan.tech_stack_and_deployment} />
      </Section>
      {(plan.testing ?? []).length > 0 && (
        <Section title="测试设计">
          <Bullets items={plan.testing} />
        </Section>
      )}
    </div>
  )
}

const SEVERITY = { high: ["严重", "bad"], medium: ["中等", "warn"], low: ["轻微", "muted"] } as const

export function ReviewTab({ plan }: { plan: Plan }) {
  const r = plan.review
  return (
    <div>
      <Section title={<>独立审稿 <ReviewChip review={r} /></>}>
        {r.summary && <p className="leading-[1.7]">{r.summary}</p>}
        <p className="text-[13px] text-muted-foreground">
          审稿经常给出「未通过」，这是设计如此：审稿就是要把方案里的矛盾挑出来，这些意见就是你下一轮要想清楚的问题。
        </p>
      </Section>
      {r.issues?.length ? (
        <Section title="意见" aside={<span className="font-mono text-[13px] text-muted-foreground">{r.issues.length}</span>}>
          <Rows>
            {r.issues.map((i, n) => {
              const [label, tone] = SEVERITY[i.severity ?? "low"] ?? ["意见", "muted"]
              return (
                <div key={n} className="flex gap-2.5">
                  <div className="w-12 shrink-0"><Chip tone={tone}>{label}</Chip></div>
                  <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="font-semibold wrap-anywhere">{i.section}</span>
                    <span className="text-[13px] leading-[1.6] text-text-secondary wrap-anywhere">{i.problem}</span>
                  </div>
                </div>
              )
            })}
          </Rows>
        </Section>
      ) : null}
    </div>
  )
}
