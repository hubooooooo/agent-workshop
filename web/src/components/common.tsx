import { CircleCheckIcon, CircleXIcon, InfoIcon, MenuIcon, TriangleAlertIcon } from "lucide-react"
import { createContext, useContext, type ReactNode } from "react"

import { Button } from "@/components/ui/button"
import type { AuditSummary, Decision, ProductSummary, Settings } from "@/lib/api"
import { cn } from "@/lib/utils"

export const SettingsContext = createContext<{ settings: Settings; reload: () => Promise<void> } | null>(null)

export function useSettings() {
  const ctx = useContext(SettingsContext)
  if (!ctx) throw new Error("SettingsContext missing")
  return ctx
}

/** 应用外壳：侧边栏里的产品和检查过的项目，以及窄屏下侧边栏的开关。 */
export const ShellContext = createContext<{
  products: ProductSummary[] | null
  reloadProducts: () => Promise<void>
  audits: AuditSummary[] | null
  reloadAudits: () => Promise<void>
  openSidebar: () => void
} | null>(null)

export function useShell() {
  const ctx = useContext(ShellContext)
  if (!ctx) throw new Error("ShellContext missing")
  return ctx
}

/** 每页顶部的标题栏，和桌面版一样 52 高、下面一条细线：左边标题（15px 加粗，可带一句灰色说明），右边操作。
 *  窄屏时最左边是打开侧边栏的按钮。 */
export function TopBar({ title, sub, actions }: { title: ReactNode; sub?: ReactNode; actions?: ReactNode }) {
  const { openSidebar } = useShell()
  return (
    <header className="flex h-[52px] shrink-0 items-center gap-3 border-b ps-3 pe-3 md:ps-5">
      <Button variant="ghost" size="icon-sm" className="md:hidden" onClick={openSidebar} aria-label="打开菜单">
        <MenuIcon />
      </Button>
      <div className="flex min-w-0 flex-1 items-center gap-2.5">
        <div className="flex shrink-0 items-center gap-2 text-[15px] font-semibold">{title}</div>
        {sub && <div className="truncate text-[13px] text-muted-foreground">{sub}</div>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  )
}

/** 小号等宽标签，assistant-ui 风格的分节标题。 */
export function Eyebrow({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("font-mono text-xs tracking-wider text-muted-foreground uppercase", className)}>
      {children}
    </div>
  )
}

/** 页面骨架：一页占满主区域的高度，顶栏固定，下面的内容自己决定怎么滚动。 */
export function Screen({ children }: { children: ReactNode }) {
  return <div className="flex min-h-0 flex-1 flex-col">{children}</div>
}

/** 内容列的宽度，和桌面版对话区、输入框一样 760：窗口再宽也不铺满（ScrollBody 两边各留 28）。 */
export const READING = "max-w-[760px]"

/** 可滚动的内容区：内容居中成一列（和桌面版的 readingScroll 一样）。 */
export function ScrollBody({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className={cn("mx-auto w-full max-w-[816px] px-7 pt-6 pb-10", className)}>{children}</div>
    </div>
  )
}

// 和桌面版的 Alert（Soft）一样：底色是白底混 5% 的强调色，外圈一条 14% 的强调色细线
const CALLOUT = {
  info: { box: "bg-[#f2f8fd] shadow-[inset_0_0_0_1px_rgba(0,118,227,0.14)]", icon: InfoIcon, color: "text-[#0076e3]" },
  warn: { box: "bg-[#fffbf2] shadow-[inset_0_0_0_1px_rgba(254,180,0,0.14)]", icon: TriangleAlertIcon, color: "text-[#feb400]" },
  bad: { box: "bg-[#fef7f5] shadow-[inset_0_0_0_1px_rgba(243,88,55,0.14)]", icon: CircleXIcon, color: "text-[#f35837]" },
  ok: { box: "bg-[#f2faf5] shadow-[inset_0_0_0_1px_rgba(0,160,63,0.14)]", icon: CircleCheckIcon, color: "text-[#00a03f]" },
} as const

/** 提示条：和桌面版的 Alert 一样，浅色底 + 细边框 + 左边图标，标题加粗、说明灰色；可带一个按钮。 */
export function Callout({
  tone = "info",
  title,
  children,
  action,
  className,
}: {
  tone?: keyof typeof CALLOUT
  title: ReactNode
  children?: ReactNode
  action?: ReactNode
  className?: string
}) {
  const { box, icon: Icon, color } = CALLOUT[tone]
  return (
    <div className={cn("mb-5 flex flex-col gap-2", className)}>
      <div className={cn("flex gap-2.5 rounded-lg", children ? "px-3.5 py-3" : "px-3 py-2.5", box)}>
        <Icon className={cn("mt-px shrink-0", children ? "size-[18px]" : "size-4", color)} />
        <div className="min-w-0 flex-1 leading-5">
          <div className="text-sm font-medium">{title}</div>
          {children && <div className="text-[13px] text-text-secondary">{children}</div>}
        </div>
      </div>
      {action && <div className="flex gap-2">{action}</div>}
    </div>
  )
}

/** 没配模型 / 离线演示时的提示；各页面顶部共用。 */
export function ModeCallouts() {
  const { settings } = useSettings()
  if (settings.mock) {
    return (
      <Callout tone="warn" title="离线演示模式">
        不调用模型，回复和方案都是按关键词拼出来的占位内容，用来看看流程长什么样，不能用于真实决策。
      </Callout>
    )
  }
  if (!settings.configured) {
    return (
      <Callout
        tone="warn"
        title="第一步：配置模型"
        action={<Button size="sm" nativeButton={false} render={<a href="#/settings" />}>去配置</Button>}
      >
        引擎要靠大模型来对话和推导方案。推荐 DeepSeek，便宜、国内可直连，充值几块钱就能用很久。
      </Callout>
    )
  }
  return null
}

/** 细边框圆角卡片（和桌面版的 card 一样）。 */
export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("flex flex-col gap-2.5 rounded-xl border bg-card px-[18px] py-4", className)}>{children}</div>
}

export const DECISIONS: { key: Decision; tone: "ok" | "warn" | "ask" | "muted"; hint: string }[] = [
  { key: "必选", tone: "ok", hint: "第一版就要做" },
  { key: "二期", tone: "warn", hint: "以后再做" },
  { key: "待确认", tone: "ask", hint: "取决于你对待确认问题的回答" },
  { key: "放弃", tone: "muted", hint: "这个产品用不上" },
]

const TONE = {
  ok: "bg-ok-soft text-ok",
  warn: "bg-warn-soft text-[#feb400]",
  ask: "bg-ask-soft text-ask",
  muted: "bg-muted text-text-secondary",
  bad: "bg-bad-soft text-destructive",
  ink: "bg-foreground text-background",
} as const

export function Chip({ tone = "muted", children, className }: { tone?: keyof typeof TONE; children: ReactNode; className?: string }) {
  return (
    <span className={cn("inline-flex h-[22px] items-center rounded-[6px] px-2 text-xs whitespace-nowrap", TONE[tone], className)}>
      {children}
    </span>
  )
}

export function timeAgo(ts: number) {
  const s = Math.max(0, Date.now() / 1000 - ts)
  if (s < 60) return "刚刚"
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  const d = new Date(ts * 1000)
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日`
}
