import { Link } from "react-router";
import { Pin } from "lucide-react";
import type { NoteSummary } from "@/lib/types";
import { formatDate } from "@/lib/utils";

export function PostList({ items }: { items: NoteSummary[] }) {
  return (
    <ul className="divide-y divide-border">
      {items.map((n) => (
        <li key={n.id} className="py-6 first:pt-0">
          <article>
            <div className="mb-1.5 flex items-center gap-2 text-xs text-fg-subtle">
              {n.pinned && (
                <span className="inline-flex items-center gap-0.5 text-primary">
                  <Pin className="h-3 w-3" /> 置顶
                </span>
              )}
              <time dateTime={n.publishedAt ?? undefined}>{formatDate(n.publishedAt)}</time>
            </div>
            <h2 className="text-lg font-semibold leading-snug">
              <Link to={`/n/${n.slug}`} className="hover:text-primary">
                {n.title}
              </Link>
            </h2>
            {n.snippet ? (
              <p className="mt-2 text-sm leading-relaxed text-fg-muted" dangerouslySetInnerHTML={{ __html: n.snippet }} />
            ) : (
              n.summary && <p className="mt-2 text-sm leading-relaxed text-fg-muted">{n.summary}</p>
            )}
            {n.tags.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-2">
                {n.tags.map((t) => (
                  <Link
                    key={t}
                    to={`/tags/${encodeURIComponent(t)}`}
                    className="rounded bg-muted px-2 py-0.5 text-xs text-fg-muted hover:text-primary"
                  >
                    #{t}
                  </Link>
                ))}
              </div>
            )}
          </article>
        </li>
      ))}
    </ul>
  );
}

export function Pager({ page, total, pageSize, onPage }: { page: number; total: number; pageSize: number; onPage: (p: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (pages <= 1) return null;
  return (
    <div className="mt-8 flex items-center justify-between text-sm">
      <button
        className="rounded-md px-3 py-1.5 text-fg-muted hover:bg-muted disabled:invisible"
        disabled={page <= 1}
        onClick={() => onPage(page - 1)}
      >
        ← 上一页
      </button>
      <span className="text-fg-subtle">
        {page} / {pages}
      </span>
      <button
        className="rounded-md px-3 py-1.5 text-fg-muted hover:bg-muted disabled:invisible"
        disabled={page >= pages}
        onClick={() => onPage(page + 1)}
      >
        下一页 →
      </button>
    </div>
  );
}
