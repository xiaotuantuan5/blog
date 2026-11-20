import { Link, NavLink, Outlet, ScrollRestoration } from "react-router";
import { LayoutDashboard, Rss, Search } from "lucide-react";
import { useMe, useSite } from "@/lib/queries";
import { ThemeToggle } from "@/components/ThemeToggle";
import { cn } from "@/lib/utils";

const nav = [
  { to: "/", label: "首页", end: true },
  { to: "/archive", label: "归档" },
  { to: "/tags", label: "标签" },
  { to: "/about", label: "关于" },
];

export function PublicLayout() {
  const { data: site } = useSite();
  const { data: me } = useMe();

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-30 border-b border-border bg-bg/85 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-5xl items-center gap-2 px-4 sm:gap-4">
          <Link to="/" className="min-w-0 truncate text-base font-semibold">
            {site?.siteTitle ?? ""}
          </Link>
          <nav className="ml-auto flex shrink-0 items-center text-sm sm:gap-1">
            {nav.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) =>
                  cn(
                    "whitespace-nowrap rounded-md px-1.5 py-1.5 transition-colors sm:px-2.5",
                    isActive ? "text-primary" : "text-fg-muted hover:text-fg",
                  )
                }
              >
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="flex shrink-0 items-center">
            <Link to="/search" className="rounded-md p-2 text-fg-muted hover:bg-muted hover:text-fg" aria-label="搜索" title="搜索">
              <Search className="h-4 w-4" />
            </Link>
            <ThemeToggle />
            {me?.loggedIn && (
              <Link to="/app" className="rounded-md p-2 text-fg-muted hover:bg-muted hover:text-fg" title="工作台" aria-label="工作台">
                <LayoutDashboard className="h-4 w-4" />
              </Link>
            )}
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8">
        <Outlet />
      </main>

      <footer className="border-t border-border py-6 text-center text-xs text-fg-subtle">
        <div className="flex items-center justify-center gap-3">
          <span>
            © {new Date().getFullYear()} {site?.author || site?.siteTitle}
          </span>
          <a href="/rss.xml" className="inline-flex items-center gap-1 hover:text-fg">
            <Rss className="h-3 w-3" /> RSS
          </a>
          {!me?.loggedIn && (
            <Link to="/login" className="hover:text-fg">
              登录
            </Link>
          )}
        </div>
      </footer>
      <ScrollRestoration />
    </div>
  );
}
