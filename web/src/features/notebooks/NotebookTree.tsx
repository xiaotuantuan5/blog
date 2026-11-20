import { useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Folder, FolderOpen, Inbox, MoreHorizontal, Plus } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { keys, useInvalidateNotes, useNotebooks } from "@/lib/queries";
import { cn } from "@/lib/utils";
import {
  Button,
  ConfirmDialog,
  MenuContent,
  MenuItem,
  MenuRoot,
  MenuSeparator,
  MenuTrigger,
  PromptDialog,
} from "@/components/ui";
import { buildTree, DND_NOTE, DND_NOTEBOOK, type NotebookNode } from "./tree";

const EXPANDED_KEY = "notebooks:expanded";

function loadExpanded(): Set<number> {
  try {
    return new Set(JSON.parse(localStorage.getItem(EXPANDED_KEY) ?? "[]"));
  } catch {
    return new Set();
  }
}

type DialogState =
  | { kind: "create"; parentId: number | null }
  | { kind: "rename"; node: NotebookNode }
  | { kind: "delete"; node: NotebookNode }
  | null;

/** Handles drops of notes or notebooks onto a notebook (or the root). */
function useDropHandler() {
  const qc = useQueryClient();
  const invalidate = useInvalidateNotes();
  const { data: notebooks = [] } = useNotebooks();
  return async (e: React.DragEvent, target: number | null) => {
    e.preventDefault();
    const noteId = Number(e.dataTransfer.getData(DND_NOTE));
    const nbId = Number(e.dataTransfer.getData(DND_NOTEBOOK));
    try {
      if (noteId) {
        await api.patchNote(noteId, { notebookId: target });
        qc.invalidateQueries({ queryKey: keys.note(noteId) });
        invalidate();
        toast.success("已移动笔记");
      } else if (nbId && nbId !== target) {
        const nb = notebooks.find((n) => n.id === nbId);
        if (!nb || nb.parentId === target) return;
        await api.updateNotebook(nbId, { name: nb.name, parentId: target, sort: nb.sort });
        invalidate();
        toast.success("已移动笔记本");
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "移动失败");
    }
  };
}

export function NotebookTree({ onNavigate }: { onNavigate?: () => void }) {
  const { data: notebooks = [] } = useNotebooks();
  const tree = useMemo(() => buildTree(notebooks), [notebooks]);
  const [expanded, setExpanded] = useState(loadExpanded);
  const [dialog, setDialog] = useState<DialogState>(null);
  const [rootOver, setRootOver] = useState(false);
  const [sp] = useSearchParams();
  const current = sp.get("notebook");
  const invalidate = useInvalidateNotes();
  const onDrop = useDropHandler();

  const toggle = (id: number, open?: boolean) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (open ?? !next.has(id)) next.add(id);
      else next.delete(id);
      localStorage.setItem(EXPANDED_KEY, JSON.stringify([...next]));
      return next;
    });
  };

  const accepts = (e: React.DragEvent) => e.dataTransfer.types.includes(DND_NOTE) || e.dataTransfer.types.includes(DND_NOTEBOOK);

  return (
    <div>
      <div className="mb-1 flex items-center justify-between px-2">
        <span className="text-xs font-semibold text-fg-subtle">笔记本</span>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          title="新建笔记本"
          aria-label="新建笔记本"
          onClick={() => setDialog({ kind: "create", parentId: null })}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>

      <Link
        to="/app/notes?notebook=0"
        onClick={onNavigate}
        onDragOver={(e) => {
          if (accepts(e)) {
            e.preventDefault();
            setRootOver(true);
          }
        }}
        onDragLeave={() => setRootOver(false)}
        onDrop={(e) => {
          setRootOver(false);
          onDrop(e, null);
        }}
        className={cn(
          "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm",
          current === "0" ? "bg-muted text-fg" : "text-fg-muted hover:bg-muted/60",
          rootOver && "ring-2 ring-primary",
        )}
        title="没有放进笔记本的笔记；把笔记本拖到这里可以移到最外层"
      >
        <Inbox className="h-4 w-4 shrink-0" />
        <span className="truncate">未归类</span>
      </Link>

      {tree.map((n) => (
        <TreeNode
          key={n.id}
          node={n}
          expanded={expanded}
          toggle={toggle}
          current={current}
          onMenu={setDialog}
          onDrop={onDrop}
          onNavigate={onNavigate}
        />
      ))}

      {tree.length === 0 && <p className="px-2 py-1 text-xs text-fg-subtle">点击 + 新建笔记本</p>}

      <PromptDialog
        open={dialog?.kind === "create"}
        onOpenChange={(o) => !o && setDialog(null)}
        title={dialog?.kind === "create" && dialog.parentId ? "新建子笔记本" : "新建笔记本"}
        placeholder="名称"
        confirmText="创建"
        onSubmit={async (name) => {
          if (dialog?.kind !== "create") return;
          await api.createNotebook(name, dialog.parentId);
          if (dialog.parentId) toggle(dialog.parentId, true);
          invalidate();
        }}
      />
      <PromptDialog
        open={dialog?.kind === "rename"}
        onOpenChange={(o) => !o && setDialog(null)}
        title="重命名笔记本"
        initial={dialog?.kind === "rename" ? dialog.node.name : ""}
        confirmText="保存"
        onSubmit={async (name) => {
          if (dialog?.kind !== "rename") return;
          const nb = dialog.node;
          await api.updateNotebook(nb.id, { name, parentId: nb.parentId, sort: nb.sort });
          invalidate();
        }}
      />
      <ConfirmDialog
        open={dialog?.kind === "delete"}
        onOpenChange={(o) => !o && setDialog(null)}
        title={`删除笔记本“${dialog?.kind === "delete" ? dialog.node.name : ""}”？`}
        description="只能删除空的笔记本。笔记本里的笔记需要先移走或删除。"
        confirmText="删除"
        danger
        onConfirm={async () => {
          if (dialog?.kind !== "delete") return;
          await api.deleteNotebook(dialog.node.id);
          invalidate();
        }}
      />
    </div>
  );
}

function TreeNode({
  node,
  expanded,
  toggle,
  current,
  onMenu,
  onDrop,
  onNavigate,
}: {
  node: NotebookNode;
  expanded: Set<number>;
  toggle: (id: number, open?: boolean) => void;
  current: string | null;
  onMenu: (d: DialogState) => void;
  onDrop: (e: React.DragEvent, target: number | null) => void;
  onNavigate?: () => void;
}) {
  const open = expanded.has(node.id);
  const active = current === String(node.id);
  const [over, setOver] = useState(false);
  const hasChildren = node.children.length > 0;
  const Icon = open && hasChildren ? FolderOpen : Folder;

  return (
    <div>
      <div
        draggable
        onDragStart={(e) => {
          e.dataTransfer.setData(DND_NOTEBOOK, String(node.id));
          e.dataTransfer.effectAllowed = "move";
        }}
        onDragOver={(e) => {
          if (e.dataTransfer.types.includes(DND_NOTE) || e.dataTransfer.types.includes(DND_NOTEBOOK)) {
            e.preventDefault();
            setOver(true);
          }
        }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => {
          setOver(false);
          onDrop(e, node.id);
          toggle(node.id, true);
        }}
        className={cn(
          "group flex items-center rounded-md text-sm",
          active ? "bg-muted text-fg" : "text-fg-muted hover:bg-muted/60",
          over && "ring-2 ring-primary",
        )}
        style={{ paddingLeft: node.depth * 12 }}
      >
        <button
          type="button"
          className={cn("flex h-7 w-5 shrink-0 items-center justify-center text-fg-subtle", !hasChildren && "invisible")}
          onClick={() => toggle(node.id)}
          aria-label={open ? "收起" : "展开"}
        >
          <ChevronRight className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-90")} />
        </button>
        <Link to={`/app/notes?notebook=${node.id}`} onClick={onNavigate} className="flex min-w-0 flex-1 items-center gap-2 py-1.5">
          <Icon className="h-4 w-4 shrink-0" />
          <span className="truncate">{node.name}</span>
          <span className="ml-auto pr-1 text-xs text-fg-subtle group-hover:hidden">{node.totalCount || ""}</span>
        </Link>
        <MenuRoot>
          <MenuTrigger asChild>
            <button
              type="button"
              className="mr-1 hidden h-6 w-6 items-center justify-center rounded text-fg-subtle hover:bg-border group-hover:flex data-[state=open]:flex"
              aria-label="笔记本操作"
            >
              <MoreHorizontal className="h-3.5 w-3.5" />
            </button>
          </MenuTrigger>
          <MenuContent>
            <MenuItem onSelect={() => onMenu({ kind: "create", parentId: node.id })}>新建子笔记本</MenuItem>
            <MenuItem onSelect={() => onMenu({ kind: "rename", node })}>重命名</MenuItem>
            <MenuSeparator />
            <MenuItem danger onSelect={() => onMenu({ kind: "delete", node })}>
              删除
            </MenuItem>
          </MenuContent>
        </MenuRoot>
      </div>
      {open &&
        node.children.map((c) => (
          <TreeNode
            key={c.id}
            node={c}
            expanded={expanded}
            toggle={toggle}
            current={current}
            onMenu={onMenu}
            onDrop={onDrop}
            onNavigate={onNavigate}
          />
        ))}
    </div>
  );
}
