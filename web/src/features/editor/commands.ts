import { EditorSelection } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";

/** Wraps the selection (or a placeholder) with before/after, toggling if already wrapped. */
export function wrapSelection(view: EditorView, before: string, after = before, placeholder = "") {
  const { state } = view;
  const tr = state.changeByRange((range) => {
    const text = state.sliceDoc(range.from, range.to);
    const outerFrom = range.from - before.length;
    const outerTo = range.to + after.length;
    // Toggle off when the selection is already wrapped.
    if (
      outerFrom >= 0 &&
      state.sliceDoc(outerFrom, range.from) === before &&
      state.sliceDoc(range.to, outerTo) === after
    ) {
      return {
        changes: [
          { from: outerFrom, to: range.from, insert: "" },
          { from: range.to, to: outerTo, insert: "" },
        ],
        range: EditorSelection.range(outerFrom, range.to - before.length),
      };
    }
    const inner = text || placeholder;
    return {
      changes: { from: range.from, to: range.to, insert: before + inner + after },
      range: EditorSelection.range(range.from + before.length, range.from + before.length + inner.length),
    };
  });
  view.dispatch(state.update(tr, { scrollIntoView: true, userEvent: "input" }));
  view.focus();
  return true;
}

/** Toggles a prefix (e.g. "> ", "- ") on every selected line. */
export function toggleLinePrefix(view: EditorView, prefix: string | ((i: number) => string)) {
  const { state } = view;
  const changes: { from: number; to?: number; insert: string }[] = [];
  const seen = new Set<number>();
  const lines: { from: number; text: string }[] = [];
  for (const r of state.selection.ranges) {
    for (let pos = r.from; pos <= r.to; ) {
      const line = state.doc.lineAt(pos);
      if (!seen.has(line.number)) {
        seen.add(line.number);
        lines.push({ from: line.from, text: line.text });
      }
      pos = line.to + 1;
    }
  }
  const prefixFor = (i: number) => (typeof prefix === "string" ? prefix : prefix(i));
  const matcher = typeof prefix === "string" ? (t: string) => t.startsWith(prefix) : (t: string) => /^\d+\. /.test(t);
  const allHave = lines.every((l) => matcher(l.text));
  lines.forEach((l, i) => {
    if (allHave) {
      const len = typeof prefix === "string" ? prefix.length : (l.text.match(/^\d+\. /)?.[0].length ?? 0);
      changes.push({ from: l.from, to: l.from + len, insert: "" });
    } else {
      changes.push({ from: l.from, insert: prefixFor(i) });
    }
  });
  view.dispatch({ changes, userEvent: "input" });
  view.focus();
  return true;
}

/** Inserts a block on its own lines at the cursor. */
export function insertBlock(view: EditorView, block: string, cursorOffset?: number) {
  const { state } = view;
  const pos = state.selection.main.head;
  const line = state.doc.lineAt(pos);
  const needsNewlineBefore = line.text.trim() !== "" ? "\n\n" : "";
  const insert = needsNewlineBefore + block + "\n";
  const at = line.text.trim() !== "" ? line.to : line.from;
  view.dispatch({
    changes: { from: at, to: line.text.trim() !== "" ? line.to : line.to, insert },
    selection: { anchor: at + needsNewlineBefore.length + (cursorOffset ?? block.length) },
    scrollIntoView: true,
    userEvent: "input",
  });
  view.focus();
  return true;
}

export function insertText(view: EditorView, text: string) {
  view.dispatch(view.state.replaceSelection(text), { scrollIntoView: true, userEvent: "input" });
  view.focus();
}
