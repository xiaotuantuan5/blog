import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { keys, useInvalidateNotes } from "@/lib/queries";
import { relativeTime, useTitle } from "@/lib/utils";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { Button, ConfirmDialog, Empty, ErrorText, PageSpinner } from "@/components/ui";

/** Deleted notes, restorable for 30 days. */
export function TrashPage() {
  useTitle("回收站");
  const qc = useQueryClient();
  const invalidate = useInvalidateNotes();
  const trash = useQuery({ queryKey: ["trash"], queryFn: api.trash });
  const [purge, setPurge] = useState<number | null>(null);

  const restore = async (id: number) => {
    try {
      await api.restoreNote(id);
      await qc.invalidateQueries({ queryKey: ["trash"] });
      invalidate();
      toast.success("已恢复");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "恢复失败");
    }
  };

  return (
    <div className="thin-scroll flex-1 overflow-y-auto">
      <div className="mx-auto max-w-3xl px-5 py-4">
        <div className="mb-4 flex items-center gap-2">
          <SidebarToggle />
          <h1 className="text-xl font-semibold">回收站</h1>
          <span className="text-sm text-fg-subtle">删除的笔记保留 30 天，之后自动清理</span>
        </div>
        {trash.isPending && <PageSpinner />}
        {trash.error && <ErrorText error={trash.error} />}
        {trash.data && trash.data.length === 0 && <Empty icon={<Trash2 className="h-8 w-8" />} title="回收站是空的" />}
        <ul className="space-y-2">
          {trash.data?.map((n) => (
            <li key={n.id} className="flex items-center gap-3 rounded-lg border border-border p-3">
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">{n.title}</p>
                <p className="mt-0.5 text-xs text-fg-subtle">
                  {n.notebookName || "未分组"} · 删除于 {relativeTime(n.deletedAt ?? "")}
                </p>
              </div>
              <Button variant="ghost" size="sm" onClick={() => restore(n.id)}>
                <RotateCcw className="h-4 w-4" /> 恢复
              </Button>
              <Button variant="ghost" size="sm" className="text-red-600 dark:text-red-400" onClick={() => setPurge(n.id)}>
                <Trash2 className="h-4 w-4" /> 彻底删除
              </Button>
            </li>
          ))}
        </ul>
        <ConfirmDialog
          open={purge !== null}
          onOpenChange={(o) => !o && setPurge(null)}
          title="彻底删除这篇笔记？"
          description="删除后无法恢复，笔记的批注和历史版本也会一并删除。"
          confirmText="彻底删除"
          danger
          onConfirm={async () => {
            if (purge === null) return;
            await api.purgeNote(purge);
            localStorage.removeItem(`draft:${purge}`);
            qc.removeQueries({ queryKey: keys.note(purge) });
            await qc.invalidateQueries({ queryKey: ["trash"] });
            invalidate();
          }}
        />
      </div>
    </div>
  );
}
