import { useEffect, useRef } from "react";
import { useIsDark } from "@/stores/ui";
import { cn, copyText } from "@/lib/utils";

let katexPromise: Promise<typeof import("katex")> | null = null;
function loadKatex() {
  katexPromise ??= Promise.all([import("katex"), import("katex/dist/katex.min.css")]).then(([k]) => k);
  return katexPromise;
}

let mermaidId = 0;

/**
 * Renders server-sanitized note HTML, then typesets math (KaTeX), draws
 * Mermaid diagrams and adds copy buttons to code blocks. KaTeX and Mermaid
 * are only downloaded when a note uses them.
 */
export function NoteContent({ html, className }: { html: string; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const dark = useIsDark();

  useEffect(() => {
    const root = ref.current;
    if (!root) return;
    let cancelled = false;

    root.querySelectorAll("img").forEach((img) => {
      img.loading = "lazy";
    });

    root.querySelectorAll<HTMLElement>(".code-block").forEach((block) => {
      if (block.querySelector(".code-tools")) return;
      const tools = document.createElement("div");
      tools.className = "code-tools";
      const lang = block.dataset.lang;
      if (lang) {
        const label = document.createElement("span");
        label.textContent = lang;
        tools.appendChild(label);
      }
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "code-copy";
      btn.textContent = "复制";
      btn.addEventListener("click", async () => {
        const code = block.querySelector("pre")?.textContent ?? "";
        try {
          await copyText(code);
          btn.textContent = "已复制";
        } catch {
          btn.textContent = "复制失败";
        }
        setTimeout(() => (btn.textContent = "复制"), 1500);
      });
      tools.appendChild(btn);
      block.appendChild(tools);
    });

    const maths = root.querySelectorAll<HTMLElement>(".math:not([data-rendered])");
    if (maths.length) {
      loadKatex().then((katex) => {
        if (cancelled) return;
        maths.forEach((el) => {
          const tex = el.textContent ?? "";
          try {
            katex.default.render(tex, el, {
              displayMode: el.classList.contains("math-display"),
              throwOnError: false,
              output: "htmlAndMathml",
            });
            el.dataset.rendered = "1";
          } catch {
            /* leave source visible */
          }
        });
      });
    }

    const diagrams = Array.from(root.querySelectorAll<HTMLElement>("pre.mermaid"));
    if (diagrams.length) {
      import("mermaid").then(async ({ default: mermaid }) => {
        if (cancelled) return;
        mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: dark ? "dark" : "default" });
        for (const el of diagrams) {
          const src = el.dataset.source ?? el.textContent ?? "";
          el.dataset.source = src;
          try {
            const { svg } = await mermaid.render(`mermaid-${++mermaidId}`, src);
            if (cancelled) return;
            el.innerHTML = svg;
            el.dataset.processed = "1";
          } catch (e) {
            el.textContent = `Mermaid 图表有误：${e instanceof Error ? e.message : String(e)}\n\n${src}`;
            delete el.dataset.processed;
          }
        }
      });
    }

    return () => {
      cancelled = true;
    };
  }, [html, dark]);

  return <div ref={ref} className={cn("prose note-content", className)} dangerouslySetInnerHTML={{ __html: html }} />;
}
