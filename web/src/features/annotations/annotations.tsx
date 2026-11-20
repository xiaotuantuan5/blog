import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { EyeOff, Eye, MessageSquarePlus, Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { Annotation, AnnotationColor } from "@/lib/types";
import { cn } from "@/lib/utils";
import { locateAnnotation, selectionToAnchor, wrapRange, type Anchor } from "./locate";
import { Button, Dialog, Textarea } from "@/components/ui";

export const ANNOTATION_COLORS: { color: AnnotationColor; label: string; dot: string }[] = [
  { color: "yellow", label: "重点", dot: "bg-amber-300" },
  { color: "green", label: "理解", dot: "bg-emerald-300" },
  { color: "blue", label: "待查", dot: "bg-sky-300" },
  { color: "red", label: "疑问", dot: "bg-red-300" },
];

/**
 * Owns a note's annotations in reading view: fetches, re-locates and wraps
 * highlights after each render of the article, reports orphaned anchors and
 * exposes CRUD straight from the current selection.
 */
export function useNoteAnnotations(
  noteId: number,
  ref: React.RefObject<HTMLElement | null>,
  htmlKey: string,
  enabled: boolean,
) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["annotations", noteId],
    queryFn: () => api.annotations(noteId),
    enabled,
  });
  const [orphans, setOrphans] = useState<number[]>([]);
  const [hidden, setHidden] = useState(false);
  const [active, setActive] = useState<{ an: Annotation; x: number; y: number } | null>(null);
  const [pendingRelocate, setPendingRelocate] = useState<number | null>(null);
  const relocateRef = useRef<number | null>(null);
  relocateRef.current = pendingRelocate;

  const invalidate = useCallback(() => {
    qc.invalidateQueries({ queryKey: ["annotations", noteId] });
    qc.invalidateQueries({ queryKey: ["annotations"] });
  }, [qc, noteId]);

  // Re-locate and wrap highlights whenever the article or the list changes.
  useEffect(() => {
    const root = ref.current;
    if (!root || !enabled || !q.data) return;
    // Unwrap previous marks first.
    root.querySelectorAll("mark.ann").forEach((m) => m.replaceWith(...Array.from(m.childNodes)));
    const located: { an: Annotation; start: number; end: number }[] = [];
    const orph: number[] = [];
    for (const an of q.data) {
      const loc = locateAnnotation(root, an);
      if (loc) located.push({ an, start: loc.start, end: loc.end });
      else orph.push(an.id);
    }
    setOrphans(orph);
    // Sync orphan flags to the server so the overview page can show them.
    for (const an of q.data) {
      if (orph.includes(an.id) && !an.orphaned) {
        api
          .updateAnnotation(an.id, {
            exact: an.exact,
            prefix: an.prefix,
            suffix: an.suffix,
            startOffset: an.startOffset,
            endOffset: an.endOffset,
            color: an.color,
            commentMd: an.commentMd,
            orphaned: true,
          })
          .catch(() => undefined);
      }
    }
    // Wrap later ranges first: wrapping splits text nodes.
    located.sort((a, b) => b.start - a.start);
    for (const { an, start, end } of located) wrapRange(root, { start, end }, an);
  }, [q.data, htmlKey, enabled, ref]);

  // Hover/click on a highlight selects that annotation.
  useEffect(() => {
    const root = ref.current;
    if (!root || !enabled) return;
    const show = (target: Element) => {
      const mark = target.closest<HTMLElement>("mark.ann");
      if (!mark || !root.contains(mark)) return;
      const id = Number(mark.dataset.aid);
      const an = q.data?.find((a) => a.id === id);
      if (!an) return;
      const r = mark.getBoundingClientRect();
      setActive({ an, x: r.left + r.width / 2, y: r.bottom });
    };
    const onOver = (e: Event) => show(e.target as Element);
    const onClick = (e: Event) => {
      const el = e.target as Element;
      if (el.closest("mark.ann")) {
        e.preventDefault();
        show(el);
      } else if (!el.closest(".ann-popover")) {
        setActive(null);
      }
    };
    root.addEventListener("mouseover", onOver);
    root.addEventListener("click", onClick);
    return () => {
      root.removeEventListener("mouseover", onOver);
      root.removeEventListener("click", onClick);
    };
  }, [q.data, enabled, ref]);

  const create = useMutation({
    mutationFn: async (input: { color: AnnotationColor; commentMd?: string; anchor?: Anchor }) => {
      const root = ref.current;
      if (!root) throw new Error("内容未就绪");
      // Prefer an anchor captured when the dialog opened: focusing its
      // textarea collapses the live selection.
      const anchor = input.anchor ?? selectionToAnchor(root);
      if (!anchor) throw new Error("请先选中一段文字");
      const body = {
        exact: anchor.exact,
        prefix: anchor.prefix,
        suffix: anchor.suffix,
        startOffset: anchor.start,
        endOffset: anchor.end,
        color: input.color,
        commentMd: input.commentMd ?? "",
        orphaned: false,
      };
      // Pending relocate: replace that annotation's anchors instead.
      const rid = relocateRef.current;
      if (rid) {
        const old = q.data?.find((a) => a.id === rid);
        setPendingRelocate(null);
        return api.updateAnnotation(rid, { ...body, commentMd: input.commentMd ?? old?.commentMd ?? "" });
      }
      return api.createAnnotation(noteId, body);
    },
    onSuccess: () => {
      window.getSelection()?.removeAllRanges();
      invalidate();
    },
    onError: (e) => toast.error(e.message),
  });

  const update = useMutation({
    mutationFn: ({ id, ...patch }: { id: number; color?: AnnotationColor; commentMd?: string }) => {
      const old = q.data?.find((a) => a.id === id);
      if (!old) throw new Error("批注不存在");
      return api.updateAnnotation(id, {
        exact: old.exact,
        prefix: old.prefix,
        suffix: old.suffix,
        startOffset: old.startOffset,
        endOffset: old.endOffset,
        color: patch.color ?? old.color,
        commentMd: patch.commentMd ?? old.commentMd,
        orphaned: old.orphaned,
      });
    },
    onSuccess: invalidate,
    onError: (e) => toast.error(e.message),
  });

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteAnnotation(id),
    onSuccess: () => {
      setActive(null);
      invalidate();
    },
    onError: (e) => toast.error(e.message),
  });

  const jump = useCallback(
    (id: number) => {
      const root = ref.current;
      const mark = root?.querySelector(`mark.ann[data-aid="${id}"]`);
      if (!mark) return;
      mark.scrollIntoView({ behavior: "smooth", block: "center" });
      mark.classList.add("ann-flash");
      setTimeout(() => mark.classList.remove("ann-flash"), 1200);
    },
    [ref],
  );

  return {
    annotations: q.data ?? [],
    orphans,
    hidden,
    setHidden,
    active,
    setActive,
    create,
    update,
    remove,
    jump,
    pendingRelocate,
    setPendingRelocate,
  };
}

/** Floating toolbar above the current selection: colors, comment, quote. */
export function SelectionToolbar({
  contentRef,
  hidden,
  onToggleHidden,
  onColor,
  onComment,
  onQuote,
  pendingRelocate,
  onCancelRelocate,
}: {
  contentRef: React.RefObject<HTMLElement | null>;
  hidden: boolean;
  onToggleHidden: () => void;
  onColor: (c: AnnotationColor) => void;
  onComment: () => void;
  onQuote: () => void;
  pendingRelocate: number | null;
  onCancelRelocate: () => void;
}) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const [hasSelection, setHasSelection] = useState(false);

  useEffect(() => {
    const onSel = () => {
      const sel = window.getSelection();
      if (!sel || sel.rangeCount === 0 || sel.isCollapsed) {
        setHasSelection(false);
        return;
      }
      const range = sel.getRangeAt(0);
      const root = contentRef.current;
      const container = range.commonAncestorContainer;
      const el = container.nodeType === Node.TEXT_NODE ? container.parentElement : (container as HTMLElement);
      if (!root || !el || !root.contains(el)) {
        setHasSelection(false);
        return;
      }
      const r = range.getBoundingClientRect();
      if (r.width === 0 && r.height === 0) return;
      setPos({ x: r.left + r.width / 2, y: r.top });
      setHasSelection(true);
    };
    document.addEventListener("selectionchange", onSel);
    window.addEventListener("scroll", () => setHasSelection(false), { passive: true, capture: true });
    return () => document.removeEventListener("selectionchange", onSel);
  }, [contentRef]);

  if (pendingRelocate) {
    return (
      <div className="ann-popover fixed bottom-6 left-1/2 z-40 -translate-x-1/2 rounded-full border border-border bg-bg px-4 py-2 text-sm shadow-lg">
        重新定位中：请在正文中选中新的文字
        <button type="button" className="ml-3 text-fg-subtle underline" onClick={onCancelRelocate}>
          取消
        </button>
      </div>
    );
  }

  if (!pos || !hasSelection) return null;
  return (
    <div
      className="ann-popover fixed z-40 flex -translate-x-1/2 -translate-y-full items-center gap-1 rounded-full border border-border bg-bg px-2 py-1.5 shadow-lg"
      style={{ left: pos.x, top: pos.y - 6 }}
    >
      {ANNOTATION_COLORS.map((c) => (
        <button
          key={c.color}
          type="button"
          title={`高亮 · ${c.label}`}
          aria-label={`高亮 · ${c.label}`}
          onClick={() => onColor(c.color)}
          className={cn("h-5 w-5 rounded-full border border-black/10 hover:scale-110 dark:border-white/20", c.dot)}
        />
      ))}
      <span className="mx-1 h-4 w-px bg-border" />
      <button type="button" onClick={onComment} title="添加批注" className="rounded-full p-1.5 text-fg-muted hover:bg-muted">
        <MessageSquarePlus className="h-4 w-4" />
      </button>
      <button type="button" onClick={onQuote} title="复制引用" className="rounded-full px-1.5 text-xs text-fg-muted hover:bg-muted">
        引用
      </button>
      <span className="mx-1 h-4 w-px bg-border" />
      <button
        type="button"
        onClick={onToggleHidden}
        title={hidden ? "显示高亮" : "隐藏高亮"}
        className="rounded-full p-1.5 text-fg-muted hover:bg-muted"
      >
        {hidden ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
      </button>
    </div>
  );
}

/** Hover/pinned popover for one highlight. */
export function AnnotationPopover({
  active,
  onClose,
  onEdit,
  onDelete,
}: {
  active: { an: Annotation; x: number; y: number } | null;
  onClose: () => void;
  onEdit: (an: Annotation) => void;
  onDelete: (id: number) => void;
}) {
  if (!active) return null;
  const { an } = active;
  return (
    <div
      className="ann-popover fixed z-40 w-72 -translate-x-1/2 rounded-lg border border-border bg-bg p-3 text-sm shadow-xl"
      style={{ left: Math.min(Math.max(active.x, 150), window.innerWidth - 150), top: active.y + 8 }}
    >
      <div className="flex items-center gap-2">
        <span className={cn("h-2.5 w-2.5 rounded-full", ANNOTATION_COLORS.find((c) => c.color === an.color)?.dot)} />
        <span className="text-xs text-fg-subtle">{ANNOTATION_COLORS.find((c) => c.color === an.color)?.label}</span>
        <div className="ml-auto flex gap-1">
          <button type="button" onClick={() => onEdit(an)} title="编辑" className="rounded p-1 text-fg-muted hover:bg-muted">
            <Pencil className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => onDelete(an.id)}
            title="删除"
            className="rounded p-1 text-red-600 hover:bg-muted dark:text-red-400"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
          <button type="button" onClick={onClose} className="rounded px-1 text-xs text-fg-subtle hover:bg-muted">
            关
          </button>
        </div>
      </div>
      {an.commentHtml ? (
        <div className="ann-comment mt-2 text-fg-muted" dangerouslySetInnerHTML={{ __html: an.commentHtml }} />
      ) : (
        <p className="mt-2 text-xs text-fg-subtle">无批注文字，点击编辑添加。</p>
      )}
    </div>
  );
}

/** Create/edit dialog for color and comment. */
export function AnnotationEditDialog({
  initial,
  title = "编辑批注",
  onOpenChange,
  onSubmit,
}: {
  initial: { color: AnnotationColor; commentMd: string; exact: string } | null;
  title?: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (patch: { color: AnnotationColor; commentMd: string }) => void;
}) {
  const [color, setColor] = useState<AnnotationColor>("yellow");
  const [comment, setComment] = useState("");
  useEffect(() => {
    if (initial) {
      setColor(initial.color);
      setComment(initial.commentMd);
    }
  }, [initial]);
  if (!initial) return null;
  return (
    <Dialog
      open
      onOpenChange={onOpenChange}
      title={title}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              onSubmit({ color, commentMd: comment });
              onOpenChange(false);
            }}
          >
            保存
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="flex items-center gap-3">
          {ANNOTATION_COLORS.map((c) => (
            <button
              key={c.color}
              type="button"
              onClick={() => setColor(c.color)}
              title={c.label}
              className={cn(
                "flex items-center gap-1.5 rounded-full border px-2 py-1 text-xs",
                color === c.color ? "border-primary" : "border-border",
              )}
            >
              <span className={cn("h-3 w-3 rounded-full", c.dot)} />
              {c.label}
            </button>
          ))}
        </div>
        <p className="rounded-md bg-muted px-3 py-2 text-xs text-fg-muted">
          “{initial.exact.slice(0, 120)}
          {initial.exact.length > 120 ? "…" : ""}”
        </p>
        <Textarea
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          rows={4}
          maxLength={5000}
          placeholder="批注内容（支持 Markdown、公式和代码）"
        />
      </div>
    </Dialog>
  );
}

/** Side panel listing this note's annotations in reading order. */
export function AnnotationsPanel({
  ann,
}: {
  ann: ReturnType<typeof useNoteAnnotations>;
}) {
  const [colorFilter, setColorFilter] = useState<AnnotationColor | "">("");
  const [editing, setEditing] = useState<Annotation | null>(null);
  const orphanItems = ann.annotations.filter((a) => ann.orphans.includes(a.id));
  const items = ann.annotations.filter((a) => !ann.orphans.includes(a.id) && (!colorFilter || a.color === colorFilter));
  const cls = useColorClass();

  return (
    <section className="space-y-2">
      <div className="flex items-center gap-1">
        <h3 className="text-xs font-semibold text-fg-subtle">划线批注 · {ann.annotations.length}</h3>
        <div className="ml-auto flex items-center gap-1">
          {(["", ...ANNOTATION_COLORS.map((c) => c.color)] as (AnnotationColor | "")[]).map((c) => (
            <button
              key={c || "all"}
              type="button"
              title={c ? ANNOTATION_COLORS.find((x) => x.color === c)?.label : "全部"}
              onClick={() => setColorFilter(c)}
              className={cn(
                "h-3 w-3 rounded-full border",
                c ? cls(c) : "border-border bg-transparent",
                colorFilter === c && "ring-2 ring-primary ring-offset-1",
              )}
            />
          ))}
          <button
            type="button"
            onClick={() => ann.setHidden(!ann.hidden)}
            title={ann.hidden ? "显示高亮" : "隐藏高亮"}
            className="ml-1 rounded p-1 text-fg-subtle hover:bg-muted"
          >
            {ann.hidden ? <Eye className="h-3.5 w-3.5" /> : <EyeOff className="h-3.5 w-3.5" />}
          </button>
        </div>
      </div>
      <ul className="space-y-2">
        {items.map((a) => (
          <li key={a.id} className="rounded-md border border-border p-2 text-xs">
            <button type="button" className="block w-full text-left" onClick={() => ann.jump(a.id)}>
              <span className={cn("mr-1 inline-block h-2 w-2 rounded-full align-middle", cls(a.color))} />
              <span className="text-fg-muted">{a.exact.slice(0, 80)}{a.exact.length > 80 ? "…" : ""}</span>
            </button>
            {a.commentHtml && (
              <div className="ann-comment mt-1 text-fg-subtle" dangerouslySetInnerHTML={{ __html: a.commentHtml }} />
            )}
            <div className="mt-1 flex gap-2 text-fg-subtle">
              <button type="button" className="hover:text-fg" onClick={() => setEditing(a)}>
                编辑
              </button>
              <button type="button" className="hover:text-red-600" onClick={() => ann.remove.mutate(a.id)}>
                删除
              </button>
            </div>
          </li>
        ))}
      </ul>
      {orphanItems.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold text-fg-subtle">失效批注 · {orphanItems.length}</h4>
          <ul className="space-y-1.5">
            {orphanItems.map((a) => (
              <li key={a.id} className="rounded-md border border-dashed border-border p-2 text-xs text-fg-subtle">
                <p className="line-clamp-2">原文已改动，找不到：{a.exact.slice(0, 80)}</p>
                <div className="mt-1 flex gap-2">
                  <button
                    type="button"
                    className="text-primary hover:underline"
                    onClick={() => {
                      ann.setActive(null);
                      ann.setPendingRelocate(a.id);
                      toast.info("请在正文中选中新的位置");
                    }}
                  >
                    重新定位
                  </button>
                  <button type="button" className="hover:text-red-600" onClick={() => ann.remove.mutate(a.id)}>
                    删除
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
      {ann.annotations.length > 0 && (
        <Link to="/app/annotations" className="block text-xs text-primary hover:underline">
          查看全部批注 →
        </Link>
      )}
      <AnnotationEditDialog
        initial={editing ? { color: editing.color, commentMd: editing.commentMd, exact: editing.exact } : null}
        onOpenChange={(o) => !o && setEditing(null)}
        onSubmit={({ color, commentMd }) => {
          if (editing) ann.update.mutate({ id: editing.id, color, commentMd });
        }}
      />
    </section>
  );
}

function useColorClass() {
  return (c: AnnotationColor) => ANNOTATION_COLORS.find((x) => x.color === c)?.dot ?? "bg-amber-300";
}
