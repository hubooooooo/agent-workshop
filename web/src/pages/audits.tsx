import { FileTextIcon, FolderSearchIcon } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { toast } from "sonner"

import { Callout, Card, Chip, ModeCallouts, Screen, ScrollBody, TopBar, timeAgo, useSettings, useShell } from "@/components/common"
import { AUDIT_STAGES, AUDIT_WORDING, JobProgress, useJob } from "@/components/job-progress"
import { Markdown } from "@/components/markdown"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { api, type AuditDetail, type AuditMode, type AuditSummary, type Job } from "@/lib/api"
import { auditHref } from "@/lib/route"
import { cn } from "@/lib/utils"

/** 删掉表头含 header 的那张表的最后一列。 */
function dropLastColumn(md: string, header: string) {
  let inTable = false
  return md
    .split("\n")
    .map((line) => {
      if (line.startsWith("|") && line.includes(`| ${header} |`)) inTable = true
      else if (!line.startsWith("|")) inTable = false
      return inTable ? line.replace(/\|[^|]*\|\s*$/, "|") : line
    })
    .join("\n")
}

/** 报告文件终端和网页共用，网页上做三处删减：大标题和「一句话判定」上面的摘要卡片已经有了；
 *  「去哪看」是引擎里组件教程的路径，给用命令行的开发者看的，网页上点不开，还会把表格挤窄。 */
function reportBody(report: string) {
  const body = report.replace(/^# .*\n+/, "").replace(/## 一句话判定\n[\s\S]*?(?=\n## )/, "")
  return dropLastColumn(body, "去哪看")
}

const pct = (x: number | null) => (x == null ? "" : `${Math.round(x * 100)}%`)

// 项目检查页和桌面版一样：左边检查过的项目，右边一份报告；「检查一个项目」在弹窗里。
// 路由：#/audits 没选中，#/a/<目录名> 选中一个，#/audits/new 打开新建弹窗。

const openNew = () => {
  location.hash = "#/audits/new"
}

export function AuditsPage({ slug, tab, newOpen }: { slug?: string; tab: string; newOpen: boolean }) {
  const { audits } = useShell()
  const closeNew = () => {
    location.hash = slug ? auditHref(slug) : "#/audits"
  }
  const dialog = <NewAuditDialog open={newOpen} onClose={closeNew} />

  if (!audits) return <Screen><TopBar title="检查过的项目" />{dialog}</Screen>

  if (!audits.length) {
    return (
      <Screen>
        <TopBar title="检查过的项目" />
        <ScrollBody>
          <ModeCallouts />
          <div className="flex flex-col gap-2.5 pt-6">
            <h1 className="text-[26px] font-semibold">看看一个已有的 Agent 项目还差什么</h1>
            <p className="text-[15px] text-text-secondary">
              选一个项目文件夹，引擎让 agent 读代码，告诉你它离一个靠谱的 Agent 还差哪些机制，并反推一份它本该有的需求文档。
            </p>
          </div>
          <Button className="mt-5" onClick={openNew}>检查一个项目 →</Button>
        </ScrollBody>
        {dialog}
      </Screen>
    )
  }

  return (
    <Screen>
      {slug ? (
        <AuditDetailPane key={slug} slug={slug} tab={tab} />
      ) : (
        <>
          <TopBar
            title={<>检查过的项目 <span className="font-mono text-xs font-normal text-muted-foreground">{audits.length}</span></>}
            actions={<Button size="sm" onClick={openNew}>检查一个项目</Button>}
          />
          <div className="flex min-h-0 flex-1">
            <AuditList audits={audits} />
            <div className="hidden min-w-0 flex-1 flex-col items-center justify-center gap-2 p-6 text-center md:flex">
              <FileTextIcon className="mb-1.5 size-8 text-text-quaternary" />
              <div className="text-[17px] font-semibold">选一个检查结果</div>
              <p className="max-w-[400px] text-muted-foreground">左边选一个项目，这里显示它的差距报告。</p>
            </div>
          </div>
        </>
      )}
      {dialog}
    </Screen>
  )
}

/** 左栏：检查过的项目（和桌面版一样 260 宽，名字 + 判定 + 覆盖率）。窄屏时没选中才显示，占满宽度。 */
function AuditList({ audits, active, className }: { audits: AuditSummary[]; active?: string; className?: string }) {
  return (
    <div className={cn("flex w-full shrink-0 flex-col gap-1 border-e px-2 py-3 md:w-[260px]", className)}>
      <div className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto">
        {audits.map((a) => (
          <a
            key={a.slug}
            href={auditHref(a.slug)}
            title={a.root}
            className={cn(
              "flex min-h-[46px] items-center gap-2.5 rounded-lg px-3 py-1.5 transition-colors hover:bg-black/[0.04]",
              active === a.slug && "bg-secondary hover:bg-secondary",
            )}
          >
            <FolderSearchIcon className="size-4 shrink-0 text-text-secondary" />
            <span className="min-w-0 flex-1">
              <span className="block truncate">{a.name}</span>
              <span className="block truncate text-xs text-muted-foreground">{!a.finished ? "没跑完" : a.verdict || "已检查"}</span>
            </span>
            {a.completion != null && <span className="shrink-0 text-xs text-muted-foreground">{pct(a.completion)}</span>}
          </a>
        ))}
      </div>
      <p className="p-2 text-[11px] text-muted-foreground">
        检查结果在引擎目录的 out/audit-项目名/ 下；在终端里用 blueprint audit 检查过的项目也会列在这里。
      </p>
    </div>
  )
}

// -- 新建（弹窗） ---------------------------------------------------------------

const NOTES = [
  ["只读，不改你的代码", "也不会运行它。agent 只能读这个文件夹里的文件。"],
  ["代码会发给你配置的模型服务商", "agent 读到的代码会随请求发出去；.env 和私钥文件不读。"],
  ["大约 3–8 分钟", "最多调用模型 40 次。中途断了可以接着跑，已完成的步骤不重复花钱。"],
]

function NewAuditDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { settings } = useSettings()
  const { reloadAudits } = useShell()
  const [path, setPath] = useState("")
  const [picking, setPicking] = useState(false)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState("")

  const pick = async () => {
    setPicking(true)
    setError("")
    try {
      const { path: chosen } = await api.pickFolder()
      if (chosen) setPath(chosen)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPicking(false)
    }
  }

  const start = async () => {
    setStarting(true)
    setError("")
    try {
      const { slug } = await api.createAudit(path)
      await reloadAudits()
      setPath("")
      location.hash = auditHref(slug)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setStarting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>检查一个已有的 Agent 项目</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3.5">
          <div className="text-[13px] font-medium">项目文件夹</div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <Button variant="outline" onClick={pick} disabled={picking || starting} className="shrink-0">
              {picking ? "在弹出的窗口里选…" : "选择文件夹…"}
            </Button>
            <Input
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="或者把文件夹路径粘贴到这里"
              spellCheck={false}
            />
          </div>
          <p className="text-xs text-muted-foreground">粘贴路径：在 Finder 里按住 Option 键右键文件夹，选「将…拷贝为路径名称」。</p>
          {NOTES.map(([title, desc]) => (
            <div key={title} className="flex flex-col gap-0.5">
              <div className="text-[13px] font-medium">{title}</div>
              <p className="text-xs text-muted-foreground">{desc}</p>
            </div>
          ))}
          {error && <p className="text-[13px] text-bad">{error}</p>}
          <div className="flex justify-end">
            <Button onClick={start} disabled={!path.trim() || starting || !settings.configured}>
              {starting ? "正在开始…" : "开始检查"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

// -- 报告 --------------------------------------------------------------------

function AuditDetailPane({ slug, tab }: { slug: string; tab: string }) {
  const { audits, reloadAudits } = useShell()
  const { settings } = useSettings()
  const [audit, setAudit] = useState<AuditDetail | null>(null)
  const [error, setError] = useState("")
  const [confirmRejudge, setConfirmRejudge] = useState(false)

  const load = useCallback(async () => {
    try {
      setAudit(await api.audit(slug))
      setError("")
    } catch (err) {
      setError((err as Error).message)
    }
    reloadAudits()
  }, [slug, reloadAudits])

  useEffect(() => {
    load()
  }, [load])

  const onFinish = useCallback(
    (job: Job) => {
      if (job.status === "done") toast.success("检查完成，报告已更新")
      load()
    },
    [load],
  )
  const job = useJob(audit?.job ?? null, onFinish)
  const running = job?.status === "running"

  const run = async (mode: AuditMode) => {
    setConfirmRejudge(false)
    try {
      await api.rerunAudit(slug, mode)
      await load()
    } catch (err) {
      toast.error((err as Error).message)
    }
  }

  const blocked = running || !settings.configured
  const active = tab === "prd" ? "prd" : "report"

  return (
    <>
      <TopBar
        title={audit ? <span className="truncate">{audit.name}</span> : "检查过的项目"}
        sub={audit && <span className="font-mono text-xs" title={audit.root}>{audit.root}</span>}
        actions={
          <>
            {audit && (
              <Button
                variant="outline"
                size="sm"
                className="hidden sm:inline-flex"
                onClick={() => api.revealAudit(slug).catch((e: Error) => toast.error(e.message))}
                title={audit.path}
              >
                打开文件夹
              </Button>
            )}
            {audit?.finished && (
              <>
                <Button variant="outline" size="sm" disabled={blocked} onClick={() => setConfirmRejudge(true)}>
                  重新判断
                </Button>
                <Button variant="outline" size="sm" disabled={blocked} onClick={() => run("recheck")} title="代码改过之后再查一次">
                  重新检查
                </Button>
              </>
            )}
            <Button size="sm" onClick={openNew}>检查一个项目</Button>
          </>
        }
      />
      <div className="flex min-h-0 flex-1">
        <AuditList audits={audits ?? []} active={slug} className="hidden md:flex" />
        <div className="flex min-w-0 flex-1 flex-col">
          <ScrollBody>
            <ModeCallouts />
            {error ? (
              <Callout tone="bad" title={error} />
            ) : (
              <>
                {job && job.status !== "done" && (
                  <div className="mb-5">
                    <JobProgress job={job} onResume={() => run("resume")} stages={AUDIT_STAGES} wording={AUDIT_WORDING} />
                  </div>
                )}
                {!audit ? (
                  <p className="text-[13px] text-muted-foreground">读取中…</p>
                ) : !audit.finished ? (
                  !job && (
                    <Callout
                      title="上次检查没跑完"
                      action={<Button size="sm" onClick={() => run("resume")} disabled={!settings.configured}>继续检查</Button>}
                    >
                      接着上次中断的地方跑，已完成的步骤不会重复调用模型。
                    </Callout>
                  )
                ) : (
                  <div className="flex flex-col gap-5">
                    {running && <Callout tone="info" className="mb-0" title="下面还是上一次的报告">这次检查完成后会自动刷新。</Callout>}
                    <Summary audit={audit} />
                    <Segmented
                      value={active}
                      options={[["report", "差距报告"], ["prd", "本该有的需求文档"]]}
                      onChange={(v) => (location.hash = auditHref(slug, v))}
                    />
                    {active === "report" ? (
                      <Markdown text={reportBody(audit.report)} />
                    ) : (
                      <>
                        <p className="text-[13px] text-muted-foreground">
                          按代码反推出来的需求文档：这个项目如果从头写需求，大概应该写成这样。缺的机制按差距报告补进去了。
                        </p>
                        <Markdown text={audit.suggested_prd} />
                      </>
                    )}
                  </div>
                )}
              </>
            )}
          </ScrollBody>
        </div>
      </div>

      <Dialog open={confirmRejudge} onOpenChange={setConfirmRejudge}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>重新判断这个项目该有什么？</DialogTitle>
            <DialogDescription className="leading-[1.7] text-foreground">
              「重新检查」会沿用上次对「这个项目该有哪些机制」的判断，结果只随代码变化。重新判断会让模型从头再判断一次，
              结果可能和上次有一两处不同。项目本身的定位变了（比如加了新功能）时再用它。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmRejudge(false)}>取消</Button>
            <Button onClick={() => run("rejudge")}>重新判断</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

/** 分段切换（和桌面版的 Segmented 一样：浅灰底，选中的一段白底）。 */
export function Segmented<T extends string>({ value, options, onChange }: { value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <div className="inline-flex self-start rounded-lg bg-secondary p-0.5">
      {options.map(([key, label]) => (
        <button
          key={key}
          type="button"
          onClick={() => onChange(key)}
          className={cn(
            "h-6 rounded-[6px] px-3 text-xs transition-colors",
            value === key ? "bg-background font-medium text-foreground shadow-xs" : "text-text-secondary hover:text-foreground",
          )}
        >
          {label}
        </button>
      ))}
    </div>
  )
}

function Summary({ audit }: { audit: AuditDetail }) {
  const facts = [
    audit.mode === "agent" ? `agent 读了 ${audit.files_seen} 个文件` : "快速扫描（按关键词判断，可能误判）",
    audit.features_from ? `「该有什么」沿用 ${audit.features_from} 那次的判断` : "",
    audit.calls ? `调用模型 ${audit.calls} 次` : "",
    timeAgo(audit.updated),
  ].filter(Boolean)
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2.5">
        <h2 className="text-xl font-semibold">{audit.verdict || "已检查"}</h2>
        <span className="text-[13px] text-muted-foreground">必选组件信号覆盖率 {pct(audit.completion)}</span>
      </div>
      {audit.title && audit.title !== audit.name && <p className="text-text-secondary">{audit.title}</p>}
      <div className="flex flex-wrap gap-1.5">
        {audit.missing > 0 && <Chip tone="bad">缺 {audit.missing} 项</Chip>}
        {audit.weak > 0 && <Chip tone="warn">弱 {audit.weak} 项</Chip>}
        {!audit.missing && !audit.weak && <Chip tone="ok">该有的都有</Chip>}
      </div>
      {audit.mode !== "agent" && audit.mode_note && <p className="text-[13px] text-warn">没用上 agent：{audit.mode_note}</p>}
      <p className="text-xs text-muted-foreground">{facts.join(" · ")}</p>
    </Card>
  )
}
