import { useEffect, useState } from "react";
import type { Heading } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Table of contents that highlights the heading currently in view. */
export function Toc({ toc, scrollRoot, className }: { toc: Heading[]; scrollRoot?: HTMLElement | null; className?: string }) {
  const [active, setActive] = useState<string>("");

  useEffect(() => {
    if (!toc.length) return;
    const els = toc.map((h) => document.getElementById(h.id)).filter((e): e is HTMLElement => !!e);
    const obs = new IntersectionObserver(
      (entries) => {
        const visible = entries.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setActive(visible[0].target.id);
      },
      { root: scrollRoot ?? null, rootMargin: "0px 0px -70% 0px" },
    );
    els.forEach((e) => obs.observe(e));
    return () => obs.disconnect();
  }, [toc, scrollRoot]);

  if (!toc.length) return null;
  const minLevel = Math.min(...toc.map((h) => h.level));

  return (
    <nav className={cn("text-sm", className)} aria-label="目录">
      <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-fg-subtle">目录</p>
      <ul className="space-y-1 border-l border-border">
        {toc.map((h) => (
          <li key={h.id}>
            <a
              href={`#${h.id}`}
              onClick={(e) => {
                e.preventDefault();
                document.getElementById(h.id)?.scrollIntoView({ behavior: "smooth", block: "start" });
                history.replaceState(null, "", `#${h.id}`);
                setActive(h.id);
              }}
              className={cn(
                "-ml-px block border-l py-0.5 pr-2 leading-snug transition-colors",
                active === h.id
                  ? "border-primary text-primary"
                  : "border-transparent text-fg-muted hover:border-fg-subtle hover:text-fg",
              )}
              style={{ paddingLeft: `${(h.level - minLevel) * 12 + 12}px` }}
            >
              {h.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
