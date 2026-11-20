import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useBlocker, useLocation, useParams, useSearchParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactCodeMirrorRef } from "@uiw/react-codemirror";
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  Columns2,
  Eye,
  ExternalLink,
  Folder,
  Globe,
  Inbox,
  Lock,
  MoreHorizontal,
  PenLine,
  Settings2,
} from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { keys, useInvalidateNotes, useNotebooks } from "@/lib/queries";
import type { Note, NoteInput, Visibility } from "@/lib/types";
import { cn, formatDateTime, useTitle } from "@/lib/utils";
import { useUI, type EditorMode } from "@/stores/ui";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { NoteContent } from "@/components/NoteContent";
import { Toc } from "@/components/Toc";
import {
  Button,
  Dialog,
  Empty,
  ErrorText,
  Field,
  Input,
  MenuContent,
  MenuItem,
  MenuRoot,
  MenuTrigger,
  PageSpinner,
  Textarea,
} from "@/components/ui";
import { MarkdownEditor } from "@/features/editor/MarkdownEditor";
import { Toolbar } from "@/features/editor/Toolbar";
import { TagInput } from "@/features/editor/TagInput";
import { NoteActions } from "@/features/notes/NoteActions";
import { UNTITLED } from "@/features/notes/useCreateNote";
import { buildTree, flattenTree, notebookPath } from "@/features/notebooks/tree";

interface Form {
  title: string;
  contentMd: string;
  tags: string[];
  visibility: Visibility;
  slug: string;
  summary: string;
}

interface Draft {
  form: Form;
  version: number;
  savedAt: number;
}

type Status = "saved" | "dirty" | "saving" | "error" | "conflict";

const AUTOSAVE_MS = 1000;
const draftKey = (id: number) => `draft:${id}`;

function fromNote(n: Note): Form {
  return {
    title: n.title === UNTITLED ? "" : n.title,
    contentMd: n.contentMd,
    tags: n.tags,
    visibility: n.visibility,
    slug: n.slug,
    summary: n.manualSummary,
  };
}

function sameForm(a: Form, b: Form) {
  return (
    a.title === b.title &&
    a.contentMd === b.contentMd &&
    a.visibility === b.visibility &&
    a.slug === b.slug &&
    a.summary === b.summary &&
    a.tags.join("\u0000") === b.tags.join("\u0000")
  );
}

function readDraft(id: number): Draft | null {
  try {
    const raw = localStorage.getItem(draftKey(id));
    return raw ? (JSON.parse(raw) as Draft) : null;
  } catch {
    return null;
  }
}

export function NoteEditorPage() {
  const { id } = useParams();
  const noteId = Number(id);
  const q = useQuery({
    queryKey: keys.note(noteId),
    queryFn: () => api.note(noteId),
    enabled: noteId > 0,
    refetchOnWindowFocus: false,
    retry: (n, e) => !(e instanceof ApiError && e.status === 404) && n < 2,
  });

  if (q.isPending) return <PageSpinner />;
  if (q.error) {
    return (
      <div className="flex flex-1 flex-col">
        <div className="p-2">
          <SidebarToggle />
        </div>
        {q.error instanceof ApiError && q.error.status === 404 ? (
          <Empty title="笔记不存在或已被删除">
            <Link to="/app/notes" className="text-primary hover:underline">
              返回笔记列表
            </Link>
          </Empty>
        ) : (
          <div className="p-4">
            <ErrorText error={q.error} />
          </div>
        )}
      </div>
    );
  }
  return <Editor key={noteId} note={q.data} />;
}

function Editor({ note }: { note: Note }) {
  const qc = useQueryClient();
  const invalidate = useInvalidateNotes();
  const location = useLocation();
  const [sp] = useSearchParams();
  const { editorMode, setEditorMode, vim } = useUI();
  const editorRef = useRef<ReactCodeMirrorRef | null>(null);

  const [form, setForm] = useState<Form>(() => fromNote(note));
  const [status, setStatus] = useState<Status>("saved");
  const [saveError, setSaveError] = useState("");
  const [draft, setDraft] = useState<Draft | null>(() => {
    const d = readDraft(note.id);
    return d && !sameForm(d.form, fromNote(note)) ? d : null;
  });
  const [settingsOpen, setSettingsOpen] = useState(false);

  const formRef = useRef(form);
  formRef.current = form;
  const savedRef = useRef<Form>(fromNote(note));
  const versionRef = useRef(note.version);
  const statusRef = useRef(status);
  statusRef.current = status;
  const inflight = useRef<Promise<boolean> | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useTitle(`${form.title || UNTITLED} - 编辑`);

  const isDirty = () => !sameForm(formRef.current, savedRef.current);

  const save = useCallback(async (): Promise<boolean> => {
    clearTimeout(timer.current);
    while (inflight.current) await inflight.current;
    if (statusRef.current === "conflict") return false;
    if (!isDirty()) {
      setStatus("saved");
      return true;
    }
    const snapshot = formRef.current;
    const input: NoteInput = {
      title: snapshot.title.trim() || UNTITLED,
      contentMd: snapshot.contentMd,
      tags: snapshot.tags,
      visibility: snapshot.visibility,
      slug: snapshot.slug,
      summary: snapshot.summary,
      version: versionRef.current,
    };
    setStatus("saving");
    const run = (async () => {
      try {
        const res = await api.updateNote(note.id, input);
        versionRef.current = res.version;
        savedRef.current = snapshot;
        qc.setQueryData(keys.note(note.id), res);
        invalidate();
        setSaveError("");
        if (isDirty()) {
          setStatus("dirty");
        } else {
          setStatus("saved");
          localStorage.removeItem(draftKey(note.id));
        }
        return true;
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        setSaveError(msg);
        setStatus(e instanceof ApiError && e.status === 409 ? "conflict" : "error");
        return false;
      }
    })();
    inflight.current = run;
    const ok = await run;
    inflight.current = null;
    if (ok && isDirty()) timer.current = setTimeout(save, AUTOSAVE_MS);
    return ok;
  }, [note.id]);

  // Autosave + local draft on every change.
  useEffect(() => {
    if (!isDirty()) return;
    if (statusRef.current !== "conflict") setStatus((s) => (s === "saving" ? s : "dirty"));
    const draftTimer = setTimeout(() => {
      const d: Draft = { form: formRef.current, version: versionRef.current, savedAt: Date.now() };
      try {
        localStorage.setItem(draftKey(note.id), JSON.stringify(d));
      } catch {
        /* storage full */
      }
    }, 300);
    clearTimeout(timer.current);
    if (statusRef.current !== "conflict") timer.current = setTimeout(save, AUTOSAVE_MS);
    return () => clearTimeout(draftTimer);
  }, [form]);

  useEffect(() => () => clearTimeout(timer.current), []);

  // Warn before closing the tab with unsaved changes.
  useEffect(() => {
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (isDirty() || inflight.current) e.preventDefault();
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, []);

  // Save before navigating to another page in the app.
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      currentLocation.pathname !== nextLocation.pathname && (isDirty() || !!inflight.current),
  );
  useEffect(() => {
    if (blocker.state !== "blocked") return;
    save().then((ok) => {
      if (ok) blocker.proceed();
      else {
        toast.error("保存失败，修改已保存在本地草稿中。请处理后再离开。");
        blocker.reset();
      }
    });
  }, [blocker, save]);

  // Live preview, rendered by the server so it matches the published page.
  const [preview, setPreview] = useState({ html: note.contentHtml, toc: note.toc });
  const previewSeq = useRef(0);
  useEffect(() => {
    if (editorMode === "edit") return;
    const seq = ++previewSeq.current;
    const t = setTimeout(async () => {
      try {
        const p = await api.preview(form.contentMd);
        if (seq === previewSeq.current) setPreview(p);
      } catch {
        /* keep last preview */
      }
    }, 300);
    return () => clearTimeout(t);
  }, [form.contentMd, editorMode]);

  const update = (patch: Partial<Form>) => setForm((f) => ({ ...f, ...patch }));
  const stableSave = useCallback(() => void save(), [save]);

  const resolveConflict = async (keepMine: boolean) => {
    const latest = await api.note(note.id);
    versionRef.current = latest.version;
    if (keepMine) {
      setStatus("dirty");
      statusRef.current = "dirty";
      await save();
    } else {
      const f = fromNote(latest);
      savedRef.current = f;
      setForm(f);
      qc.setQueryData(keys.note(note.id), latest);
      localStorage.removeItem(draftKey(note.id));
      setPreview({ html: latest.contentHtml, toc: latest.toc });
      setStatus("saved");
    }
  };

  const chars = useMemo(() => form.contentMd.replace(/\s/g, "").length, [form.contentMd]);
  const listQS = sp.toString() ? `?${sp.toString()}` : "";
  const focusTitle = (location.state as { focusTitle?: boolean } | null)?.focusTitle;

  return (
    <div className="flex h-full min-h-0 w-full flex-col">
      {/* Header */}
      <header className="flex h-11 shrink-0 items-center gap-1 border-b border-border px-2">
        <Link to={`/app/notes${listQS}`} className="rounded-md p-2 text-fg-muted hover:bg-muted md:hidden" aria-label="返回列表">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <SidebarToggle className="hidden md:flex" />
        <SaveStatus status={status} error={saveError} onRetry={stableSave} updatedAt={note.updatedAt} />
        <div className="ml-auto flex items-center gap-1">
          <ModeSwitch mode={editorMode} onChange={setEditorMode} />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => update({ visibility: form.visibility === "public" ? "private" : "public" })}
            title={form.visibility === "public" ? "公开：所有人可见。点击设为私有" : "私有：仅自己可见。点击设为公开"}
            className={cn(form.visibility === "public" && "text-primary")}
          >
            {form.visibility === "public" ? <Globe className="h-4 w-4" /> : <Lock className="h-4 w-4" />}
            <span className="hidden sm:inline">{form.visibility === "public" ? "公开" : "私有"}</span>
          </Button>
          {form.visibility === "public" && savedRef.current.visibility === "public" && (
            <a
              href={`/n/${note.slug}`}
              target="_blank"
              rel="noreferrer"
              className="rounded-md p-2 text-fg-muted hover:bg-muted"
              title="查看公开页面"
              aria-label="查看公开页面"
            >
              <ExternalLink className="h-4 w-4" />
            </a>
          )}
          <Button variant="ghost" size="icon" onClick={() => setSettingsOpen(true)} title="访问地址与摘要" aria-label="笔记设置">
            <Settings2 className="h-4 w-4" />
          </Button>
          <NoteActions
            note={{ ...note, title: form.title || UNTITLED }}
            trigger={
              <Button variant="ghost" size="icon" aria-label="更多操作">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            }
          />
        </div>
      </header>

      {/* Banners */}
      {draft && (
        <div className="flex flex-wrap items-center gap-2 border-b border-amber-300 bg-amber-50 px-4 py-2 text-sm dark:border-amber-800 dark:bg-amber-950/40">
          <AlertTriangle className="h-4 w-4 text-amber-600" />
          <span>
            发现 {formatDateTime(new Date(draft.savedAt).toISOString())} 未保存的本地草稿
            {draft.version !== note.version && "（基于较早的版本，恢复后会覆盖之后的修改）"}。
          </span>
          <div className="ml-auto flex gap-2">
            <Button
              size="sm"
              variant="primary"
              onClick={() => {
                setForm(draft.form);
                setDraft(null);
              }}
            >
              恢复草稿
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                localStorage.removeItem(draftKey(note.id));
                setDraft(null);
              }}
            >
              丢弃
            </Button>
          </div>
        </div>
      )}
      {status === "conflict" && (
        <div className="flex flex-wrap items-center gap-2 border-b border-red-300 bg-red-50 px-4 py-2 text-sm dark:border-red-900 dark:bg-red-950/40">
          <AlertTriangle className="h-4 w-4 text-red-600" />
          <span>这篇笔记已在其他窗口被修改，自动保存已暂停。你的修改保存在本地草稿中。</span>
          <div className="ml-auto flex gap-2">
            <Button size="sm" variant="danger" onClick={() => resolveConflict(true)}>
              用我的内容覆盖
            </Button>
            <Button size="sm" variant="ghost" onClick={() => resolveConflict(false)}>
              放弃我的修改，加载最新版本
            </Button>
          </div>
        </div>
      )}

      {/* Title + meta */}
      <div className="shrink-0 space-y-1.5 px-5 pb-2 pt-4">
        <input
          value={form.title}
          onChange={(e) => update({ title: e.target.value })}
          placeholder={UNTITLED}
          autoFocus={focusTitle}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.nativeEvent.isComposing) {
              e.preventDefault();
              editorRef.current?.view?.focus();
            }
          }}
          className="w-full bg-transparent text-2xl font-bold outline-none placeholder:text-fg-subtle"
          aria-label="标题"
        />
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-fg-muted">
          <NotebookPicker noteId={note.id} notebookId={note.notebookId} />
          <TagInput value={form.tags} onChange={(tags) => update({ tags })} />
        </div>
      </div>

      {editorMode !== "preview" && (
        <div className="shrink-0 border-y border-border px-3 py-1">
          <Toolbar editorRef={editorRef} />
        </div>
      )}

      {/* Body */}
      <div className="flex min-h-0 flex-1">
        {editorMode !== "preview" && (
          <div className={cn("min-h-0 min-w-0 flex-1 overflow-hidden", editorMode === "split" && "lg:border-r lg:border-border")}>
            <MarkdownEditor
              value={form.contentMd}
              onChange={(contentMd) => update({ contentMd })}
              onSave={stableSave}
              vim={vim}
              editorRef={editorRef}
              autoFocus={!focusTitle}
            />
          </div>
        )}
        {editorMode !== "edit" && (
          <div
            className={cn(
              "thin-scroll min-h-0 min-w-0 flex-1 overflow-y-auto",
              editorMode === "split" && "hidden lg:block",
            )}
          >
            <div className={cn("mx-auto px-6 py-5", editorMode === "preview" ? "flex max-w-5xl gap-10" : "max-w-3xl")}>
              <div className="min-w-0 flex-1">
                {form.contentMd.trim() ? (
                  <NoteContent html={preview.html} />
                ) : (
                  <p className="text-sm text-fg-subtle">预览会显示在这里</p>
                )}
              </div>
              {editorMode === "preview" && preview.toc.length > 0 && (
                <aside className="hidden w-52 shrink-0 xl:block">
                  <div className="sticky top-4">
                    <Toc toc={preview.toc} />
                  </div>
                </aside>
              )}
            </div>
          </div>
        )}
      </div>

      <footer className="flex h-7 shrink-0 items-center gap-3 border-t border-border px-4 text-[11px] text-fg-subtle">
        <span>{chars} 字</span>
        <span>创建于 {formatDateTime(note.createdAt)}</span>
        {vim && <span className="font-mono">VIM</span>}
        <span className="ml-auto hidden sm:inline">Ctrl+S 立即保存 · 粘贴或拖入图片即可上传</span>
      </footer>

      <NoteSettingsDialog
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        form={form}
        onSave={(patch) => update(patch)}
      />
    </div>
  );
}

function SaveStatus({ status, error, onRetry, updatedAt }: { status: Status; error: string; onRetry: () => void; updatedAt: string }) {
  if (status === "saving") return <span className="px-2 text-xs text-fg-subtle">保存中…</span>;
  if (status === "dirty") return <span className="px-2 text-xs text-fg-subtle">未保存</span>;
  if (status === "error")
    return (
      <span className="flex min-w-0 items-center gap-1 px-2 text-xs text-red-600 dark:text-red-400">
        <span className="truncate" title={error}>
          保存失败：{error}
        </span>
        <button type="button" className="shrink-0 underline" onClick={onRetry}>
          重试
        </button>
      </span>
    );
  if (status === "conflict") return <span className="px-2 text-xs text-red-600 dark:text-red-400">版本冲突</span>;
  return (
    <span className="flex items-center gap-1 px-2 text-xs text-fg-subtle" title={`最后保存：${formatDateTime(updatedAt)}`}>
      <Check className="h-3 w-3" /> 已保存
    </span>
  );
}

const modes: { mode: EditorMode; icon: typeof Eye; label: string }[] = [
  { mode: "edit", icon: PenLine, label: "仅编辑" },
  { mode: "split", icon: Columns2, label: "编辑 + 预览" },
  { mode: "preview", icon: Eye, label: "仅预览" },
];

function ModeSwitch({ mode, onChange }: { mode: EditorMode; onChange: (m: EditorMode) => void }) {
  return (
    <div className="mr-1 flex rounded-md border border-border p-0.5" role="radiogroup" aria-label="编辑模式">
      {modes.map((m) => (
        <button
          key={m.mode}
          type="button"
          role="radio"
          aria-checked={mode === m.mode}
          title={m.label}
          aria-label={m.label}
          onClick={() => onChange(m.mode)}
          className={cn(
            "flex h-6 w-7 items-center justify-center rounded",
            mode === m.mode ? "bg-muted text-fg" : "text-fg-subtle hover:text-fg",
            m.mode === "split" && "hidden lg:flex",
          )}
        >
          <m.icon className="h-3.5 w-3.5" />
        </button>
      ))}
    </div>
  );
}

function NotebookPicker({ noteId, notebookId }: { noteId: number; notebookId: number | null }) {
  const { data: notebooks = [] } = useNotebooks();
  const flat = useMemo(() => flattenTree(buildTree(notebooks)), [notebooks]);
  const qc = useQueryClient();
  const invalidate = useInvalidateNotes();

  const move = async (target: number | null) => {
    try {
      await api.patchNote(noteId, { notebookId: target });
      qc.invalidateQueries({ queryKey: keys.note(noteId) });
      invalidate();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "移动失败");
    }
  };

  return (
    <MenuRoot>
      <MenuTrigger asChild>
        <button type="button" className="inline-flex items-center gap-1 rounded px-1 py-0.5 hover:bg-muted hover:text-fg">
          {notebookId ? <Folder className="h-3.5 w-3.5" /> : <Inbox className="h-3.5 w-3.5" />}
          {notebookPath(notebooks, notebookId) || "未归类"}
        </button>
      </MenuTrigger>
      <MenuContent align="start" className="max-h-80 overflow-y-auto">
        <MenuItem disabled={notebookId == null} onSelect={() => move(null)}>
          <Inbox className="h-4 w-4" /> 未归类
        </MenuItem>
        {flat.map((nb) => (
          <MenuItem
            key={nb.id}
            disabled={nb.id === notebookId}
            onSelect={() => move(nb.id)}
            style={{ paddingLeft: 8 + nb.depth * 14 }}
          >
            <Folder className="h-4 w-4" /> {nb.name}
          </MenuItem>
        ))}
        {flat.length === 0 && <p className="px-2 py-1.5 text-xs text-fg-subtle">在侧栏中新建笔记本</p>}
      </MenuContent>
    </MenuRoot>
  );
}

function NoteSettingsDialog({
  open,
  onOpenChange,
  form,
  onSave,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  form: Form;
  onSave: (patch: Partial<Form>) => void;
}) {
  const [slug, setSlug] = useState(form.slug);
  const [summary, setSummary] = useState(form.summary);
  const [error, setError] = useState("");
  useEffect(() => {
    if (open) {
      setSlug(form.slug);
      setSummary(form.summary);
      setError("");
    }
  }, [open, form.slug, form.summary]);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="访问地址与摘要"
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              const s = slug.trim().toLowerCase();
              if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(s) || s.length > 80) {
                setError("访问地址只能包含小写字母、数字和连字符，最长 80 个字符");
                return;
              }
              onSave({ slug: s, summary: summary.trim() });
              onOpenChange(false);
            }}
          >
            保存
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="访问地址" hint={<>公开后的地址：/n/{slug || "…"}</>}>
          <Input value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="例如 go-concurrency" />
        </Field>
        <Field label="摘要" hint="显示在博客列表和分享卡片中。留空时自动截取正文开头。">
          <Textarea value={summary} onChange={(e) => setSummary(e.target.value)} rows={3} maxLength={500} />
        </Field>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      </div>
    </Dialog>
  );
}
