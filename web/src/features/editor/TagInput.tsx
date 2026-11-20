import { useState } from "react";
import { X } from "lucide-react";
import { useTags } from "@/lib/queries";

export function TagInput({ value, onChange }: { value: string[]; onChange: (tags: string[]) => void }) {
  const [input, setInput] = useState("");
  const { data: all = [] } = useTags();
  const listId = "tag-suggestions";

  const add = (raw: string) => {
    const parts = raw
      .split(/[,，\s]+/)
      .map((t) => t.trim().replace(/^#/, ""))
      .filter(Boolean);
    const lower = new Set(value.map((t) => t.toLowerCase()));
    const next = [...value];
    for (const p of parts) {
      if (!lower.has(p.toLowerCase())) {
        next.push(p);
        lower.add(p.toLowerCase());
      }
    }
    if (next.length !== value.length) onChange(next);
    setInput("");
  };

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1">
      {value.map((t) => (
        <span key={t} className="inline-flex items-center gap-0.5 rounded bg-muted px-1.5 py-0.5 text-xs text-fg-muted">
          #{t}
          <button
            type="button"
            className="rounded text-fg-subtle hover:text-fg"
            onClick={() => onChange(value.filter((x) => x !== t))}
            aria-label={`移除标签 ${t}`}
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      <input
        value={input}
        list={listId}
        onChange={(e) => {
          const v = e.target.value;
          if (/[,，]$/.test(v)) add(v);
          else setInput(v);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault();
            add(input);
          } else if (e.key === "Backspace" && !input && value.length) {
            onChange(value.slice(0, -1));
          }
        }}
        onBlur={() => input.trim() && add(input)}
        placeholder={value.length ? "" : "添加标签，回车确认"}
        className="h-6 w-28 min-w-0 bg-transparent text-xs outline-none placeholder:text-fg-subtle"
      />
      <datalist id={listId}>
        {all
          .filter((t) => !value.includes(t.name))
          .map((t) => (
            <option key={t.id} value={t.name} />
          ))}
      </datalist>
    </div>
  );
}
