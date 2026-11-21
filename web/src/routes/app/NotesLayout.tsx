import { useEffect, useMemo, useRef, useState } from "react";
import { Link, Outlet, useMatch, useSearchParams } from "react-router";
import { useInfiniteQuery } from "@tanstack/react-query";
import { FileText, Globe, Pin, Plus, Search, Star, X } from "lucide-react";
import { api } from "@/lib/api";
import { keys, useNotebooks } from "@/lib/queries";
import { cn, relativeTime } from "@/lib/utils";
import { Button, Empty, ErrorText, Input, Spinner } from "@/components/ui";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { notebookPath, DND_NOTE } from "@/features/notebooks/tree";
import { NoteActions } from "@/features/notes/NoteActions";
import { useCreateNote } from "@/features/notes/useCreateNote";

const PAGE_SIZE = 30;

export function NotesLayout() {
  const [sp, setSp] = useSearchParams();
  const editing = useMatch("/app/notes/:id");
  const activeId = editing ? Number(editing.params.id) : null;
  const { data: notebooks = [] } = useNotebooks();
  const createNote = useCreateNote();

  const notebookParam = sp.get("notebook");
  const notebook = notebookParam !== null ? Number(notebookParam) : null;
  const tag = sp.get("tag") ?? "";
  const favorite = sp.get("favorite") === "1";
  const visibility = sp.get("visibility") ?? "";
  const urlQ = sp.get("q") ?? "";
  const [qInput, setQInput] = useState(urlQ);

  // Debounce the filter box into the URL.
  useEffect(() => {
    const t = setTimeout(() => {
      if (qInput.trim() === urlQ) return;
      const next = new URLSearchParams(sp);
      if (qInput.trim()) next.set("q", qInput.trim());
      else next.delete("q");
      setSp(next, { replace: true });
    }, 300);
    return () => clearTimeout(t);
  }, [qInput, urlQ, sp, setSp]);

  const filter = { notebook, tag, favorite, visibility, q: urlQ };
  const q = useInfiniteQuery({
    queryKey: [...keys.notes, "list", filter],
    queryFn: ({ pageParam }) => api.notes({ ...filter, page: pageParam, pageSize: PAGE_SIZE }),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.pageSize < last.total ? last.page + 1 : undefined),
  });
  const items = useMemo(() => q.data?.pages.flatMap((p) => p.items) ?? [], [q.data]);
  const total = q.data?.pages[0]?.total ?? 0;

  const title =
    notebook === 0
      ? "未归类"
      : notebook
        ? notebookPath(notebooks, notebook) || "笔记本"
        : tag
          ? `#${tag}`
          : favorite
            ? "收藏"
            : visibility === "public"
              ? "已公开"
              : "全部笔记";

  // Load more when the sentinel scrolls into view.
  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = sentinel.current;
    if (!el) return;
    const obs = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting && q.hasNextPage && !q.isFetchingNextPage) q.fetchNextPage();
    });
    obs.observe(el);
    return () => obs.disconnect();
  }, [q]);

  const linkQS = sp.toString() ? `?${sp.toString()}` : "";

  return (
    <div className="flex min-h-0 flex-1">
      <section
        className={cn(
          "flex w-full min-w-0 flex-col border-r border-border md:w-80 md:shrink-0",
          editing && "hidden md:flex",
        )}
      >
        <header className="space-y-2 border-b border-border px-3 py-2">
          <div className="flex h-8 items-center gap-1">
            <SidebarToggle />
            <h1 className="min-w-0 flex-1 truncate font-semibold" title={title}>
              {title}
            </h1>
            <span className="text-xs text-fg-subtle">{total} 篇</span>
            <Button
              variant="ghost"
              size="icon"
              title="新建笔记"
              aria-label="新建笔记"
              loading={createNote.isPending}
              onClick={() => createNote.mutate(notebook && notebook > 0 ? notebook : null)}
            >
              {!createNote.isPending && <Plus className="h-4 w-4" />}
            </Button>
          </div>
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-fg-subtle" />
            <Input
              value={qInput}
              onChange={(e) => setQInput(e.target.value)}
              placeholder="在此列表中搜索"
              className="h-8 pl-8 pr-7 text-sm"
            />
            {qInput && (
              <button
                type="button"
                className="absolute right-2 top-1/2 -translate-y-1/2 text-fg-subtle hover:text-fg"
                onClick={() => setQInput("")}
                aria-label="清除"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </header>

        <div className="thin-scroll flex-1 overflow-y-auto">
          {q.isPending ? (
            <div className="flex justify-center py-10">
              <Spinner />
            </div>
          ) : q.error ? (
            <div className="p-4">
              <ErrorText error={q.error} />
            </div>
          ) : items.length === 0 ? (
            <Empty icon={<FileText className="h-8 w-8" />} title={urlQ ? "没有匹配的笔记" : "这里还没有笔记"}>
              {!urlQ && (
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-2"
                  onClick={() => createNote.mutate(notebook && notebook > 0 ? notebook : null)}
                >
                  <Plus className="h-4 w-4" /> 新建笔记
                </Button>
              )}
            </Empty>
          ) : (
            <ul>
              {items.map((n) => (
                <li key={n.id}>
                  <Link
                    to={`/app/notes/${n.id}${linkQS}`}
                    draggable
                    onDragStart={(e) => {
                      e.dataTransfer.setData(DND_NOTE, String(n.id));
                      e.dataTransfer.effectAllowed = "move";
                    }}
                    className={cn(
                      "group block border-b border-border px-3 py-2.5",
                      activeId === n.id ? "bg-muted" : "hover:bg-muted/50",
                    )}
                  >
                    <div className="flex items-center gap-1.5">
                      {n.pinned && <Pin className="h-3 w-3 shrink-0 text-primary" aria-label="置顶" />}
                      <span className="min-w-0 flex-1 truncate text-sm font-medium">{n.title}</span>
                      {n.format === "html" && (
                        <span className="shrink-0 rounded border border-border px-1 text-[9px] font-semibold leading-4 text-fg-subtle">
                          HTML
                        </span>
                      )}
                      {/* Menu and dialog events bubble through React portals; keep them away from the link. */}
                      <span
                        className="opacity-0 group-hover:opacity-100 has-[[data-state=open]]:opacity-100"
                        onClick={(e) => {
                          e.stopPropagation();
                          if (e.currentTarget.contains(e.target as Node)) e.preventDefault();
                        }}
                      >
                        <NoteActions note={n} />
                      </span>
                    </div>
                    {n.snippet ? (
                      <p
                        className="mt-1 line-clamp-2 text-xs leading-relaxed text-fg-muted"
                        dangerouslySetInnerHTML={{ __html: n.snippet }}
                      />
                    ) : (
                      n.summary && <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-fg-muted">{n.summary}</p>
                    )}
                    <div className="mt-1.5 flex items-center gap-2 text-[11px] text-fg-subtle">
                      <span>{relativeTime(n.updatedAt)}</span>
                      {n.visibility === "public" && (
                        <span className="inline-flex items-center gap-0.5 text-primary">
                          <Globe className="h-3 w-3" /> 公开
                        </span>
                      )}
                      {n.favorite && <Star className="h-3 w-3 fill-amber-400 text-amber-400" aria-label="收藏" />}
                      {n.tags.slice(0, 3).map((t) => (
                        <span key={t} className="truncate">
                          #{t}
                        </span>
                      ))}
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
          <div ref={sentinel} className="h-8">
            {q.isFetchingNextPage && (
              <div className="flex justify-center py-2">
                <Spinner className="h-4 w-4" />
              </div>
            )}
          </div>
        </div>
      </section>

      <div className={cn("min-w-0 flex-1", !editing && "hidden md:flex")}>
        <Outlet />
      </div>
    </div>
  );
}

export function NoNoteSelected() {
  const createNote = useCreateNote();
  return (
    <div className="flex flex-1 items-center justify-center">
      <Empty icon={<FileText className="h-10 w-10" />} title="选择左侧的笔记，或新建一篇">
        <Button variant="primary" size="sm" className="mt-3" onClick={() => createNote.mutate(null)} loading={createNote.isPending}>
          <Plus className="h-4 w-4" /> 新建笔记
        </Button>
      </Empty>
    </div>
  );
}
