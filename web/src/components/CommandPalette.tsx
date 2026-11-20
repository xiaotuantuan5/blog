import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { CalendarDays, CornerDownLeft, FileText, Globe, Lock, Moon, Plus, Search, Sun } from "lucide-react";
import { api } from "@/lib/api";
import { keys } from "@/lib/queries";
import type { NoteSummary } from "@/lib/types";
import { todayStr } from "@/lib/utils";
import { useUI } from "@/stores/ui";
import { useCreateNote } from "@/features/notes/useCreateNote";
import { Dialog } from "@/components/ui";

interface Cmd {
  id: string;
  label: string;
  hint?: string;
  icon: React.ReactNode;
  run: () => void;
}

/**
 * Ctrl/Cmd+K palette: jump to notes by title search plus a few commands.
 * Only mounted inside the workspace.
 */
export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { theme, setTheme } = useUI();
  const createNote = useCreateNote();
  const [q, setQ] = useState("");
  const [notes, setNotes] = useState<NoteSummary[]>([]);
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) {
      setQ("");
      setNotes([]);
      setActive(0);
    }
  }, [open]);

  // Debounced title search.
  useEffect(() => {
    if (!open) return;
    const t = setTimeout(async () => {
      try {
        const page = await api.notes({ q: q.trim(), pageSize: 8 });
        setNotes(page.items);
        setActive(0);
      } catch {
        /* keep old results */
      }
    }, 150);
    return () => clearTimeout(t);
  }, [q, open]);

  const today = useMemo(() => todayStr(), []);
  const commands: Cmd[] = useMemo(
    () => [
      {
        id: "new",
        label: "新建笔记",
        icon: <Plus className="h-4 w-4" />,
        run: () => createNote.mutate(null),
      },
      {
        id: "daily",
        label: "打开今天的日记",
        hint: today,
        icon: <CalendarDays className="h-4 w-4" />,
        run: () => navigate("/app/daily"),
      },
      {
        id: "theme",
        label: theme === "dark" ? "切换到浅色主题" : "切换到深色主题",
        icon: theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />,
        run: () => {
          setTheme(theme === "dark" ? "light" : "dark");
          onOpenChange(false);
        },
      },
    ],
    [navigate, theme, setTheme, today, onOpenChange, createNote],
  );

  const filteredCmds = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return commands;
    return commands.filter((c) => c.label.toLowerCase().includes(s));
  }, [commands, q]);

  const items: Cmd[] = useMemo(() => {
    const noteCmds: Cmd[] = notes.map((n) => ({
      id: `note-${n.id}`,
      label: n.title,
      hint: n.visibility === "public" ? "公开" : "私有",
      icon: n.visibility === "public" ? <Globe className="h-4 w-4" /> : <Lock className="h-4 w-4" />,
      run: () => {
        qc.invalidateQueries({ queryKey: keys.note(n.id) });
        navigate(`/app/notes/${n.id}`);
        onOpenChange(false);
      },
    }));
    return [...noteCmds, ...filteredCmds.map((c) => ({ ...c, run: () => (c.run(), onOpenChange(false)) }))];
  }, [notes, filteredCmds, navigate, onOpenChange, qc]);

  useEffect(() => setActive(0), [items.length]);

  const pick = (i: number) => {
    const it = items[i];
    if (it) it.run();
    else if (q.trim()) {
      // No match: fall through to a full-text search page.
      navigate(`/app/search?q=${encodeURIComponent(q.trim())}`);
      onOpenChange(false);
    }
  };

  // Keep the active item in view.
  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-idx="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="快速跳转" className="top-[12%] max-w-xl translate-y-0">
      <div className="space-y-2">
        <div className="flex items-center gap-2 rounded-md border border-border px-3">
          <Search className="h-4 w-4 shrink-0 text-fg-subtle" />
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setActive((i) => Math.min(i + 1, items.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setActive((i) => Math.max(i - 1, 0));
              } else if (e.key === "Enter" && !e.nativeEvent.isComposing) {
                e.preventDefault();
                pick(active);
              }
            }}
            placeholder="搜索笔记标题，或执行命令…"
            className="h-10 w-full bg-transparent text-sm outline-none placeholder:text-fg-subtle"
            aria-label="搜索笔记或命令"
          />
        </div>
        <div ref={listRef} className="thin-scroll max-h-72 overflow-y-auto rounded-md border border-border p-1">
          {items.map((it, i) => (
            <button
              key={it.id}
              type="button"
              data-idx={i}
              onMouseMove={() => setActive(i)}
              onClick={() => pick(i)}
              className={`flex w-full items-center gap-2 rounded px-2 py-2 text-left text-sm ${
                i === active ? "bg-muted text-fg" : "text-fg-muted"
              }`}
            >
              <span className="shrink-0 text-fg-subtle">{it.icon}</span>
              <span className="min-w-0 flex-1 truncate">{it.label}</span>
              {it.hint && <span className="shrink-0 text-xs text-fg-subtle">{it.hint}</span>}
              {i === active && <CornerDownLeft className="h-3.5 w-3.5 shrink-0 text-fg-subtle" />}
            </button>
          ))}
          {items.length === 0 && (
            <p className="flex items-center gap-2 px-2 py-2 text-sm text-fg-subtle">
              <FileText className="h-4 w-4" /> 回车全文搜索“{q.trim()}”
            </p>
          )}
        </div>
        <p className="px-1 text-xs text-fg-subtle">↑↓ 选择 · 回车打开 · Esc 关闭</p>
      </div>
    </Dialog>
  );
}
