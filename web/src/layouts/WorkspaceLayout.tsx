import { useEffect } from "react";
import { Link, Navigate, Outlet, useLocation, useNavigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  Globe,
  Hash,
  LayoutDashboard,
  LogOut,
  Menu,
  NotebookText,
  PanelLeftClose,
  Plus,
  Search,
  Settings,
  Star,
} from "lucide-react";
import { toast } from "sonner";
import { api, AUTH_EXPIRED_EVENT } from "@/lib/api";
import { keys, useMe, useSite, useTags } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { useUI } from "@/stores/ui";
import { NotebookTree } from "@/features/notebooks/NotebookTree";
import { useCreateNote } from "@/features/notes/useCreateNote";
import { ThemeToggle } from "@/components/ThemeToggle";
import { Button, PageSpinner } from "@/components/ui";

export function RequireAuth() {
  const { data: me, isPending } = useMe();
  const loc = useLocation();
  const qc = useQueryClient();

  useEffect(() => {
    const onExpired = () => {
      toast.error("登录已过期，请重新登录。未保存的内容已保存在本地草稿中。");
      qc.setQueryData(keys.me, { loggedIn: false });
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(AUTH_EXPIRED_EVENT, onExpired);
  }, [qc]);

  if (isPending) return <PageSpinner />;
  if (!me?.loggedIn) return <Navigate to={`/login?next=${encodeURIComponent(loc.pathname + loc.search)}`} replace />;
  return <Outlet />;
}

function NavItem({ to, icon: Icon, label, end, onClick }: { to: string; icon: typeof Star; label: string; end?: boolean; onClick?: () => void }) {
  const loc = useLocation();
  // Treat query strings as part of the match so "全部" and "收藏" don't both light up.
  const [path, query = ""] = to.split("?");
  const active = loc.pathname === path || (!end && loc.pathname.startsWith(path + "/"));
  const isActive = active && new URLSearchParams(loc.search).toString() === new URLSearchParams(query).toString();
  return (
    <Link
      to={to}
      onClick={onClick}
      aria-current={isActive ? "page" : undefined}
      className={cn(
        "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm",
        isActive ? "bg-muted text-fg" : "text-fg-muted hover:bg-muted/60",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="truncate">{label}</span>
    </Link>
  );
}

function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { data: site } = useSite();
  const { data: tags = [] } = useTags();
  const [sp] = useSearchParams();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const setSidebarOpen = useUI((s) => s.setSidebarOpen);
  const createNote = useCreateNote();
  const currentTag = sp.get("tag");

  const logout = async () => {
    try {
      await api.logout();
    } finally {
      qc.clear();
      navigate("/login");
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-12 items-center gap-2 px-3">
        <Link to="/app" onClick={onNavigate} className="truncate text-sm font-semibold">
          {site?.siteTitle}
        </Link>
        <Button
          variant="ghost"
          size="icon"
          className="ml-auto hidden h-7 w-7 md:flex"
          onClick={() => setSidebarOpen(false)}
          title="收起侧栏"
          aria-label="收起侧栏"
        >
          <PanelLeftClose className="h-4 w-4" />
        </Button>
      </div>

      <div className="px-3 pb-2">
        <Button
          variant="primary"
          size="sm"
          className="w-full justify-center"
          loading={createNote.isPending}
          onClick={() => {
            const nb = Number(sp.get("notebook"));
            createNote.mutate(nb > 0 ? nb : null);
            onNavigate?.();
          }}
        >
          <Plus className="h-4 w-4" /> 新建笔记
        </Button>
      </div>

      <nav className="thin-scroll flex-1 space-y-4 overflow-y-auto px-2 pb-4">
        <div className="space-y-0.5">
          <NavItem to="/app" end icon={LayoutDashboard} label="工作台" onClick={onNavigate} />
          <NavItem to="/app/notes" icon={NotebookText} label="全部笔记" onClick={onNavigate} />
          <NavItem to="/app/notes?favorite=1" icon={Star} label="收藏" onClick={onNavigate} />
          <NavItem to="/app/notes?visibility=public" icon={Globe} label="已公开" onClick={onNavigate} />
          <NavItem to="/app/search" icon={Search} label="搜索" onClick={onNavigate} />
        </div>

        <NotebookTree onNavigate={onNavigate} />

        {tags.length > 0 && (
          <div>
            <p className="mb-1 px-2 text-xs font-semibold text-fg-subtle">标签</p>
            <div className="space-y-0.5">
              {tags.map((t) => (
                <Link
                  key={t.id}
                  to={`/app/notes?tag=${encodeURIComponent(t.name)}`}
                  onClick={onNavigate}
                  className={cn(
                    "flex items-center gap-2 rounded-md px-2 py-1 text-sm",
                    currentTag === t.name ? "bg-muted text-fg" : "text-fg-muted hover:bg-muted/60",
                  )}
                >
                  <Hash className="h-3.5 w-3.5 shrink-0 text-fg-subtle" />
                  <span className="truncate">{t.name}</span>
                  <span className="ml-auto text-xs text-fg-subtle">{t.count}</span>
                </Link>
              ))}
            </div>
          </div>
        )}
      </nav>

      <div className="flex items-center gap-1 border-t border-border px-2 py-2">
        <NavItem to="/app/settings" icon={Settings} label="设置" onClick={onNavigate} />
        <div className="ml-auto flex items-center">
          <a href="/" target="_blank" rel="noreferrer" className="rounded-md p-2 text-fg-muted hover:bg-muted" title="查看博客">
            <Globe className="h-4 w-4" />
          </a>
          <ThemeToggle />
          <Button variant="ghost" size="icon" onClick={logout} title="退出登录" aria-label="退出登录">
            <LogOut className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

export function WorkspaceLayout() {
  const { sidebarOpen, mobileNavOpen, setMobileNavOpen } = useUI();
  const loc = useLocation();

  useEffect(() => setMobileNavOpen(false), [loc.pathname, loc.search, setMobileNavOpen]);

  return (
    <div className="flex h-dvh overflow-hidden">
      {sidebarOpen && (
        <aside className="hidden w-60 shrink-0 border-r border-border bg-bg-soft md:block">
          <Sidebar />
        </aside>
      )}
      {mobileNavOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div className="absolute inset-0 bg-black/40" onClick={() => setMobileNavOpen(false)} />
          <aside className="absolute inset-y-0 left-0 w-72 max-w-[85vw] border-r border-border bg-bg-soft shadow-xl">
            <Sidebar onNavigate={() => setMobileNavOpen(false)} />
          </aside>
        </div>
      )}
      <div className="flex min-w-0 flex-1 flex-col">
        <Outlet />
      </div>
    </div>
  );
}

/** Opens the sidebar; shown in page headers when the sidebar is hidden. */
export function SidebarToggle({ className }: { className?: string }) {
  const { sidebarOpen, setSidebarOpen, setMobileNavOpen } = useUI();
  return (
    <Button
      variant="ghost"
      size="icon"
      className={cn(sidebarOpen && "md:hidden", className)}
      onClick={() => (window.innerWidth >= 768 ? setSidebarOpen(true) : setMobileNavOpen(true))}
      title="打开侧栏"
      aria-label="打开侧栏"
    >
      <Menu className="h-4 w-4" />
    </Button>
  );
}
