import { BoxesIcon, RefreshCwIcon, ScanSearchIcon, SlidersHorizontalIcon, SparklesIcon, XIcon, type LucideIcon } from "lucide-react"
import { useCallback, useEffect, useState } from "react"

import { SettingsContext, ShellContext } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type AuditSummary, type ProductSummary, type Settings } from "@/lib/api"
import { productHref, useRoute, type Route } from "@/lib/route"
import { cn } from "@/lib/utils"
import { AuditsPage } from "@/pages/audits"
import { HomePage } from "@/pages/home"
import { NewProductPage } from "@/pages/new-product"
import { ProductPage } from "@/pages/product"
import { SettingsPage } from "@/pages/settings"

type NavItem = { href: string; label: string; icon: LucideIcon; desc?: string; active: boolean; title?: string }

/** 左栏的一项：和桌面版的列表项一样，图标 + 名字，可带一行灰色说明；选中时浅灰底。 */
function NavLink({ item, onNavigate }: { item: NavItem; onNavigate: () => void }) {
  const Icon = item.icon
  return (
    <a
      href={item.href}
      onClick={onNavigate}
      title={item.title}
      className={cn(
        "flex items-center gap-2.5 rounded-lg px-3 text-sm transition-colors hover:bg-black/[0.04]",
        item.desc ? "min-h-[46px] py-1.5" : "h-8",
        item.active && "bg-secondary hover:bg-secondary",
      )}
    >
      <Icon className="size-4 shrink-0 text-text-secondary" />
      <span className="min-w-0 flex-1">
        <span className="block truncate">{item.label}</span>
        {item.desc && <span className="block truncate text-xs text-muted-foreground">{item.desc}</span>}
      </span>
    </a>
  )
}

function productDesc(p: ProductSummary) {
  if (!p.has_plan) return `V${p.version} · 还没有方案`
  if (!p.unknowns) return `V${p.version} · 方案已生成`
  return `V${p.version} · 待确认 ${p.unknowns}`
}

function Sidebar({
  route,
  settings,
  error,
  products,
  onRefresh,
  onNavigate,
}: {
  route: Route
  settings: Settings | null
  error: string
  products: ProductSummary[] | null
  onRefresh: () => void
  onNavigate: () => void
}) {
  const settingsDesc = !settings
    ? undefined
    : settings.mock
      ? "离线演示"
      : settings.configured
        ? settings.model || "已配置模型"
        : "未配置模型"
  const nav: NavItem[] = [
    { href: "#/", label: "产品", icon: BoxesIcon, active: route.page === "home" },
    { href: "#/new", label: "新建产品", icon: SparklesIcon, active: route.page === "new" },
    {
      href: "#/audits",
      label: "项目检查",
      icon: ScanSearchIcon,
      active: route.page === "audits" || route.page === "audit" || route.page === "audit-new",
    },
    { href: "#/settings", label: "模型设置", icon: SlidersHorizontalIcon, desc: settingsDesc, active: route.page === "settings" },
  ]
  return (
    <div className="flex h-full flex-col gap-2 p-3">
      <div className="flex h-8 shrink-0 items-center justify-between">
        <a href="#/" onClick={onNavigate} className="text-[15px] font-semibold">
          Agent 工坊
        </a>
        <Button variant="ghost" size="icon-sm" className="hidden md:inline-flex" onClick={onRefresh} aria-label="刷新产品列表" title="刷新产品列表">
          <RefreshCwIcon />
        </Button>
        <Button variant="ghost" size="icon-sm" className="md:hidden" onClick={onNavigate} aria-label="关闭菜单">
          <XIcon />
        </Button>
      </div>

      <nav className="flex flex-col gap-0.5">
        {nav.map((item) => (
          <NavLink key={item.href} item={item} onNavigate={onNavigate} />
        ))}
      </nav>

      <div className="px-2 pt-2.5 text-xs text-muted-foreground">我的产品</div>
      <div className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto">
        {products?.length === 0 && <p className="px-2 py-1 text-xs text-muted-foreground">还没有产品，去「新建产品」聊一个出来。</p>}
        {products?.map((p) => (
          <NavLink
            key={p.slug}
            onNavigate={onNavigate}
            item={{
              href: productHref(p.slug),
              label: p.name,
              icon: BoxesIcon,
              desc: productDesc(p),
              title: p.name,
              active: route.page === "product" && route.slug === p.slug,
            }}
          />
        ))}
      </div>

      <div className="flex shrink-0 items-center gap-2 px-2 pt-1.5 pb-0.5 text-xs">
        <span className={cn("size-[7px] shrink-0 rounded-full", error ? "bg-destructive" : settings ? "bg-ok" : "bg-[#feb400]")} />
        <span className={cn("line-clamp-2", error ? "text-bad" : "text-muted-foreground")}>
          {error ? "连不上引擎，确认启动界面的窗口还开着" : settings ? "引擎已连接" : "引擎启动中…"}
        </span>
      </div>
    </div>
  )
}

export default function App() {
  const route = useRoute()
  const [settings, setSettings] = useState<Settings | null>(null)
  const [products, setProducts] = useState<ProductSummary[] | null>(null)
  const [audits, setAudits] = useState<AuditSummary[] | null>(null)
  const [error, setError] = useState("")
  const [sidebarOpen, setSidebarOpen] = useState(false)

  const reload = useCallback(async () => {
    try {
      setSettings(await api.settings())
      setError("")
    } catch (err) {
      setError((err as Error).message)
    }
  }, [])

  const reloadProducts = useCallback(async () => {
    try {
      setProducts((await api.products()).products)
    } catch {
      /* 侧边栏读不到列表不影响当前页面；下次切换页面再试 */
    }
  }, [])

  const reloadAudits = useCallback(async () => {
    try {
      setAudits((await api.audits()).audits)
    } catch {
      /* 同上 */
    }
  }, [])

  useEffect(() => {
    reload()
  }, [reload])

  // 新建产品、升版本后都会换页面，换页时顺手刷新侧边栏
  const routeKey = route.page === "product" || route.page === "audit" ? `${route.page}:${route.slug}` : route.page
  useEffect(() => {
    reloadProducts()
    reloadAudits()
  }, [reloadProducts, reloadAudits, routeKey])

  const closeSidebar = () => setSidebarOpen(false)

  return (
    <TooltipProvider>
      <ShellContext.Provider value={{ products, reloadProducts, audits, reloadAudits, openSidebar: () => setSidebarOpen(true) }}>
        <div className="flex h-svh overflow-hidden bg-layout">
          <aside
            className={cn(
              "fixed inset-y-0 start-0 z-40 w-[268px] bg-sidebar transition-transform md:static md:shrink-0 md:translate-x-0",
              sidebarOpen ? "translate-x-0 shadow-lg" : "-translate-x-full",
            )}
          >
            <Sidebar
              route={route}
              settings={settings}
              error={error}
              products={products}
              onRefresh={() => {
                reloadProducts()
                reloadAudits()
              }}
              onNavigate={closeSidebar}
            />
          </aside>
          {sidebarOpen && <div className="fixed inset-0 z-30 bg-foreground/20 md:hidden" onClick={closeSidebar} />}

          {/* 右边是一张圆角白卡片，和桌面版的页面外壳一样 */}
          <main className="flex min-w-0 flex-1 flex-col overflow-hidden bg-background md:my-2 md:me-2 md:rounded-xl md:border md:border-border-strong">
            {error && <div className="mx-auto mt-10 max-w-xl px-4 text-center text-sm text-bad">{error}</div>}
            {settings && (
              <SettingsContext.Provider value={{ settings, reload }}>
                {route.page === "home" && <HomePage />}
                {route.page === "new" && <NewProductPage />}
                {route.page === "settings" && <SettingsPage />}
                {route.page === "product" && <ProductPage slug={route.slug} tab={route.tab} />}
                {(route.page === "audits" || route.page === "audit-new" || route.page === "audit") && (
                  <AuditsPage
                    slug={route.page === "audit" ? route.slug : undefined}
                    tab={route.page === "audit" ? route.tab : "report"}
                    newOpen={route.page === "audit-new"}
                  />
                )}
              </SettingsContext.Provider>
            )}
          </main>
        </div>
      </ShellContext.Provider>
      <Toaster position="bottom-center" />
    </TooltipProvider>
  )
}
