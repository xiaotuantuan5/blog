import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Globe, Search } from "lucide-react";
import { api } from "@/lib/api";
import { keys, useNotebooks } from "@/lib/queries";
import { cn, relativeTime, useTitle } from "@/lib/utils";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { Empty, ErrorText, Input, PageSpinner } from "@/components/ui";
import { notebookPath } from "@/features/notebooks/tree";
import { Pager } from "@/components/PostList";

const PAGE_SIZE = 20;

export function SearchPage() {
  const [sp, setSp] = useSearchParams();
  const urlQ = sp.get("q") ?? "";
  const visibility = sp.get("visibility") ?? "";
  const page = Math.max(1, Number(sp.get("page")) || 1);
  const [input, setInput] = useState(urlQ);
  const { data: notebooks = [] } = useNotebooks();
  useTitle("搜索");

  useEffect(() => {
    const t = setTimeout(() => {
      if (input.trim() === urlQ) return;
      const next = new URLSearchParams(sp);
      if (input.trim()) next.set("q", input.trim());
      else next.delete("q");
      next.delete("page");
      setSp(next, { replace: true });
    }, 300);
    return () => clearTimeout(t);
  }, [input, urlQ, sp, setSp]);

  const q = useQuery({
    queryKey: [...keys.notes, "search", urlQ, visibility, page],
    queryFn: () => api.notes({ q: urlQ, visibility, page, pageSize: PAGE_SIZE }),
    enabled: urlQ !== "",
    placeholderData: keepPreviousData,
  });

  const setParam = (k: string, v: string) => {
    const next = new URLSearchParams(sp);
    if (v) next.set(k, v);
    else next.delete(k);
    if (k !== "page") next.delete("page");
    setSp(next);
  };

  return (
    <div className="thin-scroll flex-1 overflow-y-auto">
      <div className="mx-auto max-w-3xl px-5 py-4">
        <div className="mb-4 flex items-center gap-2">
          <SidebarToggle />
          <h1 className="text-xl font-semibold">搜索</h1>
        </div>
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-fg-subtle" />
          <Input
            autoFocus
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="搜索标题和正文，多个关键词用空格分隔"
            className="h-11 pl-9 text-base"
          />
        </div>
        <div className="mt-3 flex gap-1 text-sm">
          {[
            ["", "全部"],
            ["private", "私有"],
            ["public", "公开"],
          ].map(([v, label]) => (
            <button
              key={v}
              type="button"
              onClick={() => setParam("visibility", v)}
              className={cn(
                "rounded-md px-2.5 py-1",
                visibility === v ? "bg-muted text-fg" : "text-fg-muted hover:bg-muted/60",
              )}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="mt-6">
          {!urlQ ? (
            <p className="text-sm text-fg-subtle">支持中文、英文和代码片段。</p>
          ) : q.isPending ? (
            <PageSpinner />
          ) : q.error ? (
            <ErrorText error={q.error} />
          ) : q.data.items.length === 0 ? (
            <Empty icon={<Search className="h-8 w-8" />} title={`没有找到“${urlQ}”`} />
          ) : (
            <>
              <p className="mb-3 text-sm text-fg-subtle">找到 {q.data.total} 篇</p>
              <ul className="space-y-2">
                {q.data.items.map((n) => (
                  <li key={n.id}>
                    <Link to={`/app/notes/${n.id}`} className="block rounded-lg border border-border p-3 hover:border-primary">
                      <div className="flex items-center gap-2">
                        <span className="min-w-0 flex-1 truncate font-medium">{n.title}</span>
                        {n.visibility === "public" && <Globe className="h-3.5 w-3.5 text-primary" aria-label="公开" />}
                      </div>
                      {n.snippet && (
                        <p className="mt-1 text-sm leading-relaxed text-fg-muted" dangerouslySetInnerHTML={{ __html: n.snippet }} />
                      )}
                      <div className="mt-2 flex flex-wrap gap-2 text-xs text-fg-subtle">
                        <span>{notebookPath(notebooks, n.notebookId) || "未归类"}</span>
                        <span>{relativeTime(n.updatedAt)}</span>
                        {n.tags.map((t) => (
                          <span key={t}>#{t}</span>
                        ))}
                      </div>
                    </Link>
                  </li>
                ))}
              </ul>
              <Pager page={page} total={q.data.total} pageSize={PAGE_SIZE} onPage={(p) => setParam("page", String(p))} />
            </>
          )}
        </div>
      </div>
    </div>
  );
}
