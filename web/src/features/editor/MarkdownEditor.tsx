import { useEffect, useMemo, useState } from "react";
import CodeMirror, { type ReactCodeMirrorRef } from "@uiw/react-codemirror";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { EditorView, keymap, type KeyBinding } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { useIsDark } from "@/stores/ui";
import type { NoteFormat } from "@/lib/types";
import { toggleLinePrefix, wrapSelection } from "./commands";
import { uploadIntoEditor } from "./upload";

export const formattingKeys: KeyBinding[] = [
  { key: "Mod-b", run: (v) => wrapSelection(v, "**", "**", "粗体") },
  { key: "Mod-i", run: (v) => wrapSelection(v, "*", "*", "斜体") },
  { key: "Mod-e", run: (v) => wrapSelection(v, "`", "`", "代码") },
  { key: "Mod-Shift-x", run: (v) => wrapSelection(v, "~~", "~~", "删除线") },
  { key: "Mod-Shift-k", run: (v) => wrapSelection(v, "[", "](https://)", "链接文字") },
  { key: "Mod-Shift-.", run: (v) => toggleLinePrefix(v, "> ") },
];

/** Same shortcuts, but wrapping in tags for notes written as HTML. */
export const htmlFormattingKeys: KeyBinding[] = [
  { key: "Mod-b", run: (v) => wrapSelection(v, "<b>", "</b>", "粗体") },
  { key: "Mod-i", run: (v) => wrapSelection(v, "<em>", "</em>", "斜体") },
  { key: "Mod-e", run: (v) => wrapSelection(v, "<code>", "</code>", "代码") },
  { key: "Mod-Shift-k", run: (v) => wrapSelection(v, '<a href="https://">', "</a>", "链接文字") },
];

const editorTheme = EditorView.theme({
  "&": { backgroundColor: "transparent" },
  ".cm-content": { caretColor: "var(--primary)" },
  ".cm-placeholder": { color: "var(--fg-subtle)" },
});

export function MarkdownEditor({
  value,
  onChange,
  onSave,
  vim,
  editorRef,
  autoFocus,
  format = "md",
}: {
  value: string;
  onChange: (v: string) => void;
  onSave: () => void;
  vim: boolean;
  editorRef: React.RefObject<ReactCodeMirrorRef | null>;
  autoFocus?: boolean;
  format?: NoteFormat;
}) {
  const dark = useIsDark();
  const [vimExt, setVimExt] = useState<Extension | null>(null);
  const [htmlLang, setHtmlLang] = useState<Extension | null>(null);

  useEffect(() => {
    if (!vim) {
      setVimExt(null);
      return;
    }
    let cancelled = false;
    import("@replit/codemirror-vim").then((m) => {
      if (!cancelled) setVimExt(m.vim());
    });
    return () => {
      cancelled = true;
    };
  }, [vim]);

  useEffect(() => {
    if (format !== "html") {
      setHtmlLang(null);
      return;
    }
    let cancelled = false;
    languages
      .find((l) => l.name === "HTML")
      ?.load()
      .then((support) => {
        if (!cancelled) setHtmlLang(support);
      });
    return () => {
      cancelled = true;
    };
  }, [format]);

  const extensions = useMemo(() => {
    const exts: Extension[] = [
      format === "html"
        ? htmlLang ?? []
        : markdown({ base: markdownLanguage, codeLanguages: languages }),
      EditorView.lineWrapping,
      editorTheme,
      keymap.of([
        {
          key: "Mod-s",
          preventDefault: true,
          run: () => {
            onSave();
            return true;
          },
        },
        ...(format === "html" ? htmlFormattingKeys : formattingKeys),
      ]),
      EditorView.domEventHandlers({
        paste(e, view) {
          const files = Array.from(e.clipboardData?.files ?? []);
          if (!files.length) return false;
          e.preventDefault();
          uploadIntoEditor(view, files, undefined, format);
          return true;
        },
        drop(e, view) {
          const files = Array.from(e.dataTransfer?.files ?? []);
          if (!files.length) return false;
          e.preventDefault();
          const pos = view.posAtCoords({ x: e.clientX, y: e.clientY }) ?? undefined;
          uploadIntoEditor(view, files, pos, format);
          return true;
        },
      }),
    ];
    // Vim must come first so its keymap takes precedence.
    if (vimExt) exts.unshift(vimExt);
    return exts;
  }, [onSave, vimExt, htmlLang, format]);

  return (
    <CodeMirror
      ref={editorRef}
      className="cm-editor-wrap h-full"
      value={value}
      onChange={onChange}
      extensions={extensions}
      theme={dark ? "dark" : "light"}
      autoFocus={autoFocus}
      placeholder={
        format === "html"
          ? "粘贴 HTML 源码，右侧实时预览；保存时会自动清洗（脚本会被移除）"
          : "开始写作……支持 Markdown，可直接粘贴或拖入图片"
      }
      basicSetup={{
        lineNumbers: false,
        foldGutter: false,
        highlightActiveLine: false,
        highlightActiveLineGutter: false,
        autocompletion: false,
        searchKeymap: true,
      }}
      height="100%"
    />
  );
}
