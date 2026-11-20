import { useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { ExternalLink, FolderInput, Inbox, MoreHorizontal, Pin, PinOff, Star, StarOff, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { keys, useInvalidateNotes, useNotebooks } from "@/lib/queries";
import type { NoteSummary } from "@/lib/types";
import {
  ConfirmDialog,
  MenuContent,
  MenuItem,
  MenuRoot,
  MenuSeparator,
  MenuSub,
  MenuSubContent,
  MenuSubTrigger,
  MenuTrigger,
} from "@/components/ui";
import { buildTree, flattenTree } from "../notebooks/tree";

/** Dropdown with pin / favorite / move / delete for a note. */
export function NoteActions({
  note,
  trigger,
  onDeleted,
}: {
  note: Pick<NoteSummary, "id" | "title" | "slug" | "pinned" | "favorite" | "visibility" | "notebookId">;
  trigger?: React.ReactNode;
  onDeleted?: () => void;
}) {
  const qc = useQueryClient();
  const invalidate = useInvalidateNotes();
  const navigate = useNavigate();
  const { data: notebooks = [] } = useNotebooks();
  const flat = useMemo(() => flattenTree(buildTree(notebooks)), [notebooks]);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const patch = async (flags: Parameters<typeof api.patchNote>[1], msg: string) => {
    try {
      await api.patchNote(note.id, flags);
      qc.invalidateQueries({ queryKey: keys.note(note.id) });
      invalidate();
      toast.success(msg);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "操作失败");
    }
  };

  return (
    <>
      <MenuRoot>
        <MenuTrigger asChild>
          {trigger ?? (
            <button
              type="button"
              className="flex h-6 w-6 items-center justify-center rounded text-fg-subtle hover:bg-border hover:text-fg"
              aria-label="笔记操作"
              onClick={(e) => e.preventDefault()}
            >
              <MoreHorizontal className="h-4 w-4" />
            </button>
          )}
        </MenuTrigger>
        <MenuContent onClick={(e) => e.stopPropagation()}>
          <MenuItem onSelect={() => patch({ pinned: !note.pinned }, note.pinned ? "已取消置顶" : "已置顶")}>
            {note.pinned ? <PinOff className="h-4 w-4" /> : <Pin className="h-4 w-4" />}
            {note.pinned ? "取消置顶" : "置顶"}
          </MenuItem>
          <MenuItem onSelect={() => patch({ favorite: !note.favorite }, note.favorite ? "已取消收藏" : "已收藏")}>
            {note.favorite ? <StarOff className="h-4 w-4" /> : <Star className="h-4 w-4" />}
            {note.favorite ? "取消收藏" : "收藏"}
          </MenuItem>
          <MenuSub>
            <MenuSubTrigger>
              <FolderInput className="h-4 w-4" /> 移动到
            </MenuSubTrigger>
            <MenuSubContent>
              <MenuItem disabled={note.notebookId == null} onSelect={() => patch({ notebookId: null }, "已移到“未归类”")}>
                <Inbox className="h-4 w-4" /> 未归类
              </MenuItem>
              {flat.map((nb) => (
                <MenuItem
                  key={nb.id}
                  disabled={nb.id === note.notebookId}
                  onSelect={() => patch({ notebookId: nb.id }, `已移到“${nb.name}”`)}
                  style={{ paddingLeft: 8 + nb.depth * 14 }}
                >
                  {nb.name}
                </MenuItem>
              ))}
            </MenuSubContent>
          </MenuSub>
          {note.visibility === "public" && (
            <MenuItem onSelect={() => window.open(`/n/${note.slug}`, "_blank", "noopener")}>
              <ExternalLink className="h-4 w-4" /> 查看公开页面
            </MenuItem>
          )}
          <MenuSeparator />
          <MenuItem danger onSelect={() => setConfirmDelete(true)}>
            <Trash2 className="h-4 w-4" /> 删除
          </MenuItem>
        </MenuContent>
      </MenuRoot>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`删除“${note.title}”？`}
        description="移入回收站，30 天内可恢复，之后自动清除。"
        confirmText="删除"
        danger
        onConfirm={async () => {
          await api.deleteNote(note.id);
          localStorage.removeItem(`draft:${note.id}`);
          qc.removeQueries({ queryKey: keys.note(note.id) });
          invalidate();
          toast.success("已移入回收站");
          if (onDeleted) onDeleted();
          else if (location.pathname === `/app/notes/${note.id}`) navigate("/app/notes");
        }}
      />
    </>
  );
}
