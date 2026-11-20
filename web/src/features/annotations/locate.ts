import type { Annotation } from "@/lib/types";

/**
 * Anchor re-location for annotations, following the W3C Web Annotation
 * idea: store the text itself (exact/prefix/suffix) instead of DOM paths,
 * then find it again in the rendered article after edits.
 */

interface TextPos {
  node: Text;
  start: number;
  end: number;
}

interface TextIndex {
  full: string;
  positions: TextPos[];
}

function buildIndex(root: HTMLElement): TextIndex {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const positions: TextPos[] = [];
  let full = "";
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const t = n as Text;
    positions.push({ node: t, start: full.length, end: full.length + t.data.length });
    full += t.data;
  }
  return { full, positions };
}

function toPoint(index: TextIndex, offset: number): { node: Text; offset: number } | null {
  for (const p of index.positions) {
    if (offset >= p.start && offset <= p.end) return { node: p.node, offset: offset - p.start };
  }
  return null;
}

/** Character offset of a DOM position inside the text stream, or -1. */
function charOf(index: TextIndex, container: Node, offset: number): number {
  if (container.nodeType === Node.TEXT_NODE) {
    for (const p of index.positions) {
      if (p.node === container) return p.start + offset;
    }
    return -1;
  }
  // Element container: the position sits between child nodes. The text
  // starts at the first text node of child[offset] when present, otherwise
  // right after the last text node before it.
  const el = container as HTMLElement;
  for (const p of index.positions) {
    if (!container.contains(p.node)) continue;
    if (offset < el.childNodes.length) {
      const boundary = el.childNodes[offset];
      // Is p.node the first text node inside boundary (or boundary itself)?
      const first = firstTextNode(boundary);
      if (first === p.node) return p.start;
    }
  }
  let last = -1;
  for (const p of index.positions) {
    if (container.contains(p.node)) last = Math.max(last, p.end);
  }
  return last;
}

function firstTextNode(node: Node): Text | null {
  if (node.nodeType === Node.TEXT_NODE) return node as Text;
  const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
  return (walker.nextNode() as Text) ?? null;
}

/** Collapses whitespace and maps normalized offsets back to raw ones. */
function normalize(s: string): { norm: string; map: number[] } {
  const map: number[] = [];
  let norm = "";
  let pendingSpace = false;
  for (let i = 0; i < s.length; i++) {
    if (/\s/.test(s[i])) {
      pendingSpace = true;
    } else {
      if (pendingSpace && norm) {
        norm += " ";
        map.push(i - 1);
      }
      norm += s[i];
      map.push(i);
      pendingSpace = false;
    }
  }
  return { norm, map };
}

function bestOccurrence(hay: string, exact: string, prefix: string, suffix: string, hint: number): number {
  const hits: number[] = [];
  let i = hay.indexOf(exact);
  while (i >= 0 && hits.length < 100) {
    hits.push(i);
    i = hay.indexOf(exact, i + 1);
  }
  if (!hits.length) return -1;
  let best = hits[0];
  let bestScore = -Infinity;
  for (const h of hits) {
    let score = 0;
    const pre = hay.slice(Math.max(0, h - prefix.length), h);
    let k = 0;
    while (k < prefix.length && k < pre.length && prefix[prefix.length - 1 - k] === pre[pre.length - 1 - k]) k++;
    score += k * 2;
    const suf = hay.slice(h + exact.length, h + exact.length + suffix.length);
    let m = 0;
    while (m < suffix.length && m < suf.length && suffix[m] === suf[m]) m++;
    score += m * 2;
    // Prefer occurrences near the recorded position.
    score -= Math.min(Math.abs(h - hint) / 500, 4);
    if (score > bestScore) {
      bestScore = score;
      best = h;
    }
  }
  return best;
}

/** Whole-block treatment for code blocks and formulas. */
function expandToBlock(index: TextIndex, start: number, end: number, root: HTMLElement): [number, number] {
  const p = toPoint(index, start);
  if (!p) return [start, end];
  const el = p.node.parentElement?.closest<HTMLElement>(".math, pre, .code-block");
  if (!el || !root.contains(el)) return [start, end];
  const blockStart = indexStartOf(index, firstTextNode(el));
  if (blockStart < 0) return [start, end];
  const blockIndex = buildIndex(el);
  return [blockStart, blockStart + blockIndex.full.length];
}

function indexStartOf(index: TextIndex, node: Text | null): number {
  if (!node) return -1;
  for (const p of index.positions) if (p.node === node) return p.start;
  return -1;
}

/** Result of a successful location: character offsets into the text stream. */
export interface Located {
  start: number;
  end: number;
}

/** Finds an annotation's anchors in the rendered root, or null. */
export function locateAnnotation(root: HTMLElement, an: Annotation, index?: TextIndex): Located | null {
  const idx = index ?? buildIndex(root);
  if (!an.exact) return null;

  const start = bestOccurrence(idx.full, an.exact, an.prefix, an.suffix, an.startOffset);
  if (start >= 0) return finish(idx, start, start + an.exact.length, root);

  // Fuzzy pass: whitespace-insensitive matching.
  const whole = normalize(idx.full);
  const nExact = normalize(an.exact).norm;
  if (!nExact || whole.map.length === 0) return null;
  const nStart = bestOccurrence(
    whole.norm,
    nExact,
    normalize(an.prefix).norm,
    normalize(an.suffix).norm,
    whole.map[Math.min(Math.max(an.startOffset, 0), whole.map.length - 1)],
  );
  if (nStart < 0 || nStart >= whole.map.length) return null;
  const rawStart = whole.map[nStart];
  const endN = nStart + nExact.length - 1;
  if (endN >= whole.map.length) return null;
  return finish(idx, rawStart, whole.map[endN] + 1, root);
}

function finish(idx: TextIndex, start: number, end: number, root: HTMLElement): Located {
  const [bs, be] = expandToBlock(idx, start, end, root);
  return { start: bs, end: be };
}

/**
 * Wraps [start, end) of the current tree in `<mark class="ann …">` elements.
 * Call in reverse document order (later ranges first) so earlier offsets
 * stay valid, because wrapping splits text nodes.
 */
export function wrapRange(root: HTMLElement, located: Located, an: Annotation) {
  const index = buildIndex(root);
  const { start, end } = located;
  if (!toPoint(index, start) || !toPoint(index, end)) return;

  const hits: { node: Text; s: number; e: number }[] = [];
  for (const p of index.positions) {
    if (p.end <= start || p.start >= end) continue;
    hits.push({ node: p.node, s: Math.max(start - p.start, 0), e: Math.min(end - p.start, p.node.data.length) });
  }
  for (const { node, s, e } of hits) {
    let target = node;
    if (e < node.data.length) target.splitText(e);
    let mid = target;
    if (s > 0) mid = target.splitText(s);
    const mark = document.createElement("mark");
    mark.className = `ann ann-${an.color}`;
    mark.dataset.aid = String(an.id);
    mid.parentNode?.insertBefore(mark, mid);
    mark.appendChild(mid);
  }
}

/** Text anchors captured from a selection. */
export interface Anchor {
  exact: string;
  prefix: string;
  suffix: string;
  start: number;
  end: number;
}

/** The current selection inside root as text anchors, or null. */
export function selectionToAnchor(root: HTMLElement): Anchor | null {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return null;
  const range = sel.getRangeAt(0);
  if (range.collapsed) return null;
  const container = range.commonAncestorContainer;
  const containerEl = container.nodeType === Node.TEXT_NODE ? container.parentElement : (container as HTMLElement);
  if (!containerEl || !root.contains(containerEl)) return null;

  const index = buildIndex(root);
  const startChar = charOf(index, range.startContainer, range.startOffset);
  const endChar = charOf(index, range.endContainer, range.endOffset);
  if (startChar < 0 || endChar <= startChar) return null;

  const exact = index.full.slice(startChar, endChar).replace(/\s+$/, "");
  if (!exact.trim()) return null;
  const prefix = index.full.slice(Math.max(0, startChar - 32), startChar);
  const suffix = index.full.slice(startChar + exact.length, startChar + exact.length + 32);
  return { exact, prefix, suffix, start: startChar, end: startChar + exact.length };
}
