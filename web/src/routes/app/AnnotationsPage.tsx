import { useState } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Highlighter } from "lucide-react";
import { api } from "@/lib/api";
import { useNotebooks } from "@/lib/queries";
import { cn, formatDate, useTitle } from "@/lib/utils";
import { ANNOTATION_COLORS } from "@/features/annotations/annotations";
import type { AnnotationColor } from "@/lib/types";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { Empty, ErrorText, Input, PageSpinner } from "@/components/ui";

/** All annotations across notes, filterable by color, notebook and text. */
export function AnnotationsPage() {
  useTitle("批注汇总");
  const [q, setQ] = useState("");
  const [color, setColor] = useState<AnnotationColor | "">("");
  const [notebook, setNotebook] = useState<number | "">("");
  const { data: notebooks = [] } = useNotebooks();
  const list = useQuery({
    queryKey: ["annotations", "all", { q, color, notebook }],
    queryFn: () =>
      api.annotationList({
        q: q.trim() || undefined,
        color: color || undefined,
        notebook: notebook === "" ? undefined : notebook,
        page: 1,
      }),
    placeholderData: (prev) => prev,
  });

  const dot = (c: AnnotationColor) => ANNOTATION_COLORS.find((x) => x.color === c)?.dot ?? "bg-amber-300";

  return (
    <div className="thin-scroll flex-1 overflow-y-auto">
      <div className="mx-auto max-w-3xl px-5 py-4">
        <div className="mb-4 flex items-center gap-2">
          <SidebarToggle />
          <h1 className="text-xl font-semibold">批注汇总</h1>
          <span className="text-sm text-fg-subtle">{list.data?.total ?? "…"} 条</span>
        </div>

        <div className="mb-4 flex flex-wrap items-center gap-2">
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="搜索批注或原文…"
            className="h-8 max-w-xs"
          />
          <div className="flex items-center gap-1.5">
            {(["", ...ANNOTATION_COLORS.map((c) => c.color)] as (AnnotationColor | "")[]).map((c) => (
              <button
                key={c || "all"}
                type="button"
                title={c ? ANNOTATION_COLORS.find((x) => x.color === c)?.label : "全部颜色"}
                onClick={() => setColor(c)}
                className={cn(
                  "h-4 w-4 rounded-full border border-black/10 dark:border-white/20",
                  c ? dot(c) : "bg-transparent",
                  color === c && "ring-2 ring-primary ring-offset-1",
                )}
              />
            ))}
          </div>
          <select
            value={notebook}
            onChange={(e) => setNotebook(e.target.value === "" ? "" : Number(e.target.value))}
            className="h-8 rounded-md border border-border bg-bg px-2 text-sm"
            aria-label="按笔记本筛选"
          >
            <option value="">全部笔记本</option>
            {notebooks.map((nb) => (
              <option key={nb.id} value={nb.id}>
                {nb.name}
              </option>
            ))}
          </select>
        </div>

        {list.isPending && <PageSpinner />}
        {list.error && <ErrorText error={list.error} />}
        {list.data && list.data.items.length === 0 && (
          <Empty icon={<Highlighter className="h-8 w-8" />} title="还没有批注">
            在笔记的阅读视图里选中文字即可高亮或添加批注。
          </Empty>
        )}
        <ul className="space-y-2">
          {list.data?.items.map((a) => (
            <li key={a.id} className="rounded-lg border border-border p-3 text-sm">
              <div className="mb-1 flex items-center gap-2 text-xs text-fg-subtle">
                <span className={cn("h-2.5 w-2.5 rounded-full", dot(a.color))} />
                <Link to={`/app/notes/${a.noteId}`} className="font-medium text-fg hover:underline">
                  {a.noteTitle}
                </Link>
                {a.orphaned && <span className="rounded bg-muted px-1.5 py-0.5">原文已改动</span>}
                <span className="ml-auto">{formatDate(a.updatedAt)}</span>
              </div>
              <p className="text-fg-muted">
                <span className="ann-quote">“{a.exact.slice(0, 160)}{a.exact.length > 160 ? "…" : ""}”</span>
              </p>
              {a.commentHtml && (
                <div className="ann-comment mt-1.5 text-fg-muted" dangerouslySetInnerHTML={{ __html: a.commentHtml }} />
              )}
            </li>
          ))}
        </ul>
        {list.data && list.data.total > list.data.items.length && (
          <p className="mt-4 text-center text-xs text-fg-subtle">
            已显示前 {list.data.items.length} 条，共 {list.data.total} 条，可用关键词缩小范围
          </p>
        )}
      </div>
    </div>
  );
}
