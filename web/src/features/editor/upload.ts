import type { EditorView } from "@codemirror/view";
import { toast } from "sonner";
import { api } from "@/lib/api";

let seq = 0;

function escapeLabel(s: string) {
  return s.replace(/[[\]]/g, "");
}

/**
 * Uploads files and inserts Markdown links at the cursor. A placeholder is
 * inserted first and replaced once the upload finishes, so typing can continue.
 */
export async function uploadIntoEditor(view: EditorView, files: File[], pos?: number) {
  for (const file of files) {
    const marker = `![上传中 ${escapeLabel(file.name)} #${++seq}]()`;
    const at = pos ?? view.state.selection.main.head;
    view.dispatch({ changes: { from: at, insert: marker + "\n" }, userEvent: "input" });
    pos = undefined;

    const replace = (text: string) => {
      const doc = view.state.doc.toString();
      const i = doc.indexOf(marker);
      if (i < 0) return;
      const end = i + marker.length + (doc[i + marker.length] === "\n" && !text ? 1 : 0);
      view.dispatch({ changes: { from: i, to: end, insert: text }, userEvent: "input" });
    };

    try {
      const f = await api.upload(file);
      const name = escapeLabel(file.name.replace(/\.[^.]+$/, "")) || "文件";
      replace(f.isImage ? `![${name}](${f.url})` : `[${escapeLabel(f.filename)}](${f.url})`);
    } catch (e) {
      replace("");
      toast.error(`${file.name} 上传失败：${e instanceof Error ? e.message : e}`);
    }
  }
}

/** Opens a file picker and uploads the chosen files into the editor. */
export function pickAndUpload(view: EditorView, accept = "image/*,.pdf,.zip,.txt,.md") {
  const input = document.createElement("input");
  input.type = "file";
  input.accept = accept;
  input.multiple = true;
  input.onchange = () => {
    const files = Array.from(input.files ?? []);
    if (files.length) uploadIntoEditor(view, files);
  };
  input.click();
}
