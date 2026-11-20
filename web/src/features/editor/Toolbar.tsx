import type { ReactCodeMirrorRef } from "@uiw/react-codemirror";
import {
  Bold,
  Code,
  FileCode2,
  Heading2,
  Image,
  Italic,
  Link2,
  List,
  ListChecks,
  ListOrdered,
  MessageSquareWarning,
  Quote,
  Sigma,
  Strikethrough,
  Table,
} from "lucide-react";
import type { EditorView } from "@codemirror/view";
import { insertBlock, toggleLinePrefix, wrapSelection } from "./commands";
import { pickAndUpload } from "./upload";

type Action = { icon: typeof Bold; label: string; run: (v: EditorView) => void } | "sep";

const actions: Action[] = [
  { icon: Heading2, label: "标题", run: (v) => toggleLinePrefix(v, "## ") },
  { icon: Bold, label: "粗体 (Ctrl+B)", run: (v) => wrapSelection(v, "**", "**", "粗体") },
  { icon: Italic, label: "斜体 (Ctrl+I)", run: (v) => wrapSelection(v, "*", "*", "斜体") },
  { icon: Strikethrough, label: "删除线 (Ctrl+Shift+X)", run: (v) => wrapSelection(v, "~~", "~~", "删除线") },
  "sep",
  { icon: Quote, label: "引用 (Ctrl+Shift+.)", run: (v) => toggleLinePrefix(v, "> ") },
  { icon: List, label: "无序列表", run: (v) => toggleLinePrefix(v, "- ") },
  { icon: ListOrdered, label: "有序列表", run: (v) => toggleLinePrefix(v, (i) => `${i + 1}. `) },
  { icon: ListChecks, label: "任务列表", run: (v) => toggleLinePrefix(v, "- [ ] ") },
  "sep",
  { icon: Code, label: "行内代码 (Ctrl+E)", run: (v) => wrapSelection(v, "`", "`", "代码") },
  { icon: FileCode2, label: "代码块", run: (v) => insertBlock(v, "```\n\n```", 4) },
  { icon: Link2, label: "链接 (Ctrl+Shift+K)", run: (v) => wrapSelection(v, "[", "](https://)", "链接文字") },
  { icon: Image, label: "上传图片或附件（也可直接粘贴、拖入）", run: (v) => pickAndUpload(v) },
  { icon: Table, label: "表格", run: (v) => insertBlock(v, "| 列 1 | 列 2 |\n| --- | --- |\n|  |  |", 2) },
  { icon: Sigma, label: "公式", run: (v) => insertBlock(v, "$$\n\n$$", 3) },
  { icon: MessageSquareWarning, label: "提示块", run: (v) => insertBlock(v, "> [!TIP]\n> ", 11) },
];

export function Toolbar({ editorRef }: { editorRef: React.RefObject<ReactCodeMirrorRef | null> }) {
  return (
    <div className="flex items-center gap-0.5 overflow-x-auto" role="toolbar" aria-label="格式">
      {actions.map((a, i) =>
        a === "sep" ? (
          <span key={i} className="mx-1 h-4 w-px shrink-0 bg-border" />
        ) : (
          <button
            key={a.label}
            type="button"
            title={a.label}
            aria-label={a.label}
            className="flex h-7 w-7 shrink-0 items-center justify-center rounded text-fg-muted hover:bg-muted hover:text-fg"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => {
              const view = editorRef.current?.view;
              if (view) a.run(view);
            }}
          >
            <a.icon className="h-4 w-4" />
          </button>
        ),
      )}
    </div>
  );
}
