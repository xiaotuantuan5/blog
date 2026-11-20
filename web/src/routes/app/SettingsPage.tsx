import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { keys, useInvalidateNotes, useTags } from "@/lib/queries";
import type { Settings, Tag } from "@/lib/types";
import { useTitle } from "@/lib/utils";
import { useUI } from "@/stores/ui";
import { SidebarToggle } from "@/layouts/WorkspaceLayout";
import { Button, ConfirmDialog, ErrorText, Field, Input, PageSpinner, PromptDialog, Textarea } from "@/components/ui";

function Section({ id, title, children }: { id?: string; title: string; children: React.ReactNode }) {
  return (
    <section id={id} className="scroll-mt-4 rounded-lg border border-border p-5">
      <h2 className="mb-4 font-semibold">{title}</h2>
      {children}
    </section>
  );
}

export function SettingsPage() {
  useTitle("设置");
  useEffect(() => {
    if (location.hash) document.getElementById(location.hash.slice(1))?.scrollIntoView();
  }, []);
  return (
    <div className="thin-scroll flex-1 overflow-y-auto">
      <div className="mx-auto max-w-2xl space-y-6 px-5 py-4 pb-16">
        <div className="flex items-center gap-2">
          <SidebarToggle />
          <h1 className="text-xl font-semibold">设置</h1>
        </div>
        <SiteSettings />
        <EditorSettings />
        <TagSettings />
        <PasswordSettings />
      </div>
    </div>
  );
}

function SiteSettings() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: keys.settings, queryFn: api.settings });
  const [form, setForm] = useState<Settings | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (q.data && !form) setForm(q.data);
  }, [q.data, form]);

  if (q.isPending || !form) return <PageSpinner />;
  if (q.error) return <ErrorText error={q.error} />;

  const set = (patch: Partial<Settings>) => setForm({ ...form, ...patch });

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const saved = await api.updateSettings(form);
      setForm(saved);
      qc.setQueryData(keys.settings, saved);
      qc.invalidateQueries({ queryKey: keys.site });
      toast.success("已保存");
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Section title="站点">
      <form onSubmit={submit} className="space-y-4">
        <Field label="站点名称">
          <Input value={form.siteTitle} onChange={(e) => set({ siteTitle: e.target.value })} required maxLength={100} />
        </Field>
        <Field label="简介">
          <Input value={form.siteDescription} onChange={(e) => set({ siteDescription: e.target.value })} />
        </Field>
        <Field label="作者">
          <Input value={form.author} onChange={(e) => set({ author: e.target.value })} />
        </Field>
        <Field label="站点地址" hint="用于 RSS、站点地图和分享链接，例如 https://blog.example.com。留空时根据访问地址自动推断。">
          <Input value={form.siteUrl} onChange={(e) => set({ siteUrl: e.target.value })} placeholder="https://" />
        </Field>
        <Field label="头像">
          <div className="flex items-center gap-3">
            {form.avatar && <img src={form.avatar} alt="" className="h-12 w-12 rounded-full object-cover" />}
            <Button variant="outline" size="sm" onClick={() => fileRef.current?.click()}>
              <Upload className="h-4 w-4" /> 上传
            </Button>
            {form.avatar && (
              <Button variant="ghost" size="sm" onClick={() => set({ avatar: "" })}>
                移除
              </Button>
            )}
            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              hidden
              onChange={async (e) => {
                const f = e.target.files?.[0];
                e.target.value = "";
                if (!f) return;
                try {
                  const up = await api.upload(f);
                  set({ avatar: up.url });
                } catch (err) {
                  toast.error(err instanceof Error ? err.message : "上传失败");
                }
              }}
            />
          </div>
        </Field>
        <Field label="每页文章数">
          <Input
            type="number"
            min={1}
            max={100}
            value={form.pageSize}
            onChange={(e) => set({ pageSize: Number(e.target.value) })}
            className="w-28"
          />
        </Field>
        <Field label="关于页面（Markdown）">
          <Textarea
            value={form.aboutMd}
            onChange={(e) => set({ aboutMd: e.target.value })}
            rows={8}
            className="font-mono text-[13px]"
          />
        </Field>
        <ErrorText error={error} />
        <Button type="submit" variant="primary" loading={busy}>
          保存
        </Button>
      </form>
    </Section>
  );
}

function EditorSettings() {
  const { vim, setVim } = useUI();
  return (
    <Section title="编辑器">
      <label className="flex items-center gap-3 text-sm">
        <input type="checkbox" checked={vim} onChange={(e) => setVim(e.target.checked)} className="h-4 w-4" />
        <span>
          Vim 键位
          <span className="ml-2 text-xs text-fg-subtle">只保存在当前浏览器</span>
        </span>
      </label>
    </Section>
  );
}

function TagSettings() {
  const { data: tags = [], isPending } = useTags();
  const invalidate = useInvalidateNotes();
  const qc = useQueryClient();
  const [renaming, setRenaming] = useState<Tag | null>(null);
  const [deleting, setDeleting] = useState<Tag | null>(null);

  const refresh = () => {
    invalidate();
    // Open notes show tags from their own query.
    qc.invalidateQueries({ queryKey: ["note"] });
  };

  return (
    <Section id="tags" title="标签">
      {isPending ? (
        <PageSpinner />
      ) : tags.length === 0 ? (
        <p className="text-sm text-fg-subtle">还没有标签。在编辑笔记时添加。</p>
      ) : (
        <ul className="divide-y divide-border">
          {tags.map((t) => (
            <li key={t.id} className="flex items-center gap-2 py-2 text-sm">
              <span className="flex-1">#{t.name}</span>
              <span className="text-xs text-fg-subtle">{t.count} 篇</span>
              <Button variant="ghost" size="icon" onClick={() => setRenaming(t)} aria-label={`重命名 ${t.name}`} title="重命名">
                <Pencil className="h-3.5 w-3.5" />
              </Button>
              <Button variant="ghost" size="icon" onClick={() => setDeleting(t)} aria-label={`删除 ${t.name}`} title="删除">
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <PromptDialog
        open={!!renaming}
        onOpenChange={(o) => !o && setRenaming(null)}
        title="重命名标签"
        label="与已有标签同名时会合并"
        initial={renaming?.name ?? ""}
        confirmText="保存"
        onSubmit={async (name) => {
          if (!renaming) return;
          await api.renameTag(renaming.id, name);
          refresh();
        }}
      />
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`删除标签“${deleting?.name ?? ""}”？`}
        description="只会从笔记上移除这个标签，不会删除笔记。"
        confirmText="删除"
        danger
        onConfirm={async () => {
          if (!deleting) return;
          await api.deleteTag(deleting.id);
          refresh();
        }}
      />
    </Section>
  );
}

function PasswordSettings() {
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (newPw !== confirm) {
      setError(new Error("两次输入的新密码不一致"));
      return;
    }
    setBusy(true);
    try {
      await api.changePassword(oldPw, newPw);
      setOldPw("");
      setNewPw("");
      setConfirm("");
      toast.success("密码已修改，其他设备已退出登录");
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Section title="修改密码">
      <form onSubmit={submit} className="space-y-4">
        <Field label="原密码">
          <Input type="password" value={oldPw} onChange={(e) => setOldPw(e.target.value)} autoComplete="current-password" required />
        </Field>
        <Field label="新密码" hint="至少 8 位">
          <Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} autoComplete="new-password" required minLength={8} />
        </Field>
        <Field label="确认新密码">
          <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" required />
        </Field>
        <ErrorText error={error} />
        <Button type="submit" variant="primary" loading={busy}>
          修改密码
        </Button>
      </form>
    </Section>
  );
}
