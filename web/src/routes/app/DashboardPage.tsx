import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { CalendarDays, FileText, FolderTree, Globe, Hash, Paperclip, Plus, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { keys, useMe, useSite } from "@/lib/queries";
import { relativeTime, useTitle } from "@/lib/utils";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { Heatmap } from "@/components/Heatmap";
import { Button, Empty, ErrorText, PageSpinner } from "@/components/ui";
import { useCreateNote } from "@/features/notes/useCreateNote";

function greeting() {
  const h = new Date().getHours();
  if (h < 6) return "夜深了";
  if (h < 12) return "早上好";
  if (h < 14) return "中午好";
  if (h < 18) return "下午好";
  return "晚上好";
}

export function DashboardPage() {
  const { data: me } = useMe();
  const { data: site } = useSite();
  const stats = useQuery({ queryKey: keys.stats, queryFn: api.stats });
  const recent = useQuery({ queryKey: [...keys.notes, "recent"], queryFn: () => api.notes({ pageSize: 10 }) });
  const createNote = useCreateNote();
  useTitle(`工作台 - ${site?.siteTitle ?? ""}`);

  const cards = stats.data
    ? [
        { label: "笔记", value: stats.data.notes, icon: FileText, to: "/app/notes" },
        { label: "已公开", value: stats.data.public, icon: Globe, to: "/app/notes?visibility=public" },
        { label: "笔记本", value: stats.data.notebooks, icon: FolderTree },
        { label: "标签", value: stats.data.tags, icon: Hash, to: "/app/settings#tags" },
        { label: "附件", value: stats.data.files, icon: Paperclip },
        { label: "回收站", value: stats.data.trash, icon: Trash2, to: "/app/trash" },
      ]
    : [];

  return (
    <div className="thin-scroll flex-1 overflow-y-auto">
      <div className="mx-auto max-w-4xl px-5 py-4">
        <div className="mb-6 flex items-center gap-2">
          <SidebarToggle />
          <h1 className="text-xl font-semibold">
            {greeting()}，{me?.username}
          </h1>
          <Button variant="primary" size="sm" className="ml-auto" onClick={() => createNote.mutate(null)} loading={createNote.isPending}>
            <Plus className="h-4 w-4" /> 新建笔记
          </Button>
        </div>

        {stats.error && <ErrorText error={stats.error} />}
        <div className="mb-4">
          <Link
            to="/app/daily"
            className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-sm hover:bg-muted"
          >
            <CalendarDays className="h-4 w-4" /> 今天的日记
          </Link>
        </div>
        <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          {cards.map((c) => {
            const body = (
              <>
                <c.icon className="h-4 w-4 text-fg-subtle" />
                <p className="mt-2 text-2xl font-semibold">{c.value}</p>
                <p className="text-xs text-fg-muted">{c.label}</p>
              </>
            );
            return c.to ? (
              <Link key={c.label} to={c.to} className="rounded-lg border border-border p-4 hover:border-primary">
                {body}
              </Link>
            ) : (
              <div key={c.label} className="rounded-lg border border-border p-4">
                {body}
              </div>
            );
          })}
        </div>

        <h2 className="mb-2 text-sm font-semibold text-fg-muted">学习热力图</h2>
        <div className="mb-8 rounded-lg border border-border p-4">
          <Heatmap />
        </div>

        <h2 className="mb-2 text-sm font-semibold text-fg-muted">最近编辑</h2>
        {recent.isPending ? (
          <PageSpinner />
        ) : recent.error ? (
          <ErrorText error={recent.error} />
        ) : recent.data.items.length === 0 ? (
          <Empty icon={<FileText className="h-8 w-8" />} title="还没有笔记">
            点击右上角“新建笔记”开始记录。
          </Empty>
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {recent.data.items.map((n) => (
              <li key={n.id}>
                <Link to={`/app/notes/${n.id}`} className="flex items-center gap-3 px-4 py-2.5 hover:bg-muted/50">
                  <span className="min-w-0 flex-1 truncate text-sm">{n.title}</span>
                  {n.visibility === "public" && <Globe className="h-3.5 w-3.5 text-primary" aria-label="公开" />}
                  <span className="shrink-0 text-xs text-fg-subtle">{relativeTime(n.updatedAt)}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
