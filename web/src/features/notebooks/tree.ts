import type { Notebook } from "@/lib/types";

export interface NotebookNode extends Notebook {
  children: NotebookNode[];
  depth: number;
  /** Notes in this notebook and all descendants. */
  totalCount: number;
}

export function buildTree(list: Notebook[]): NotebookNode[] {
  const byId = new Map<number, NotebookNode>();
  for (const nb of list) byId.set(nb.id, { ...nb, children: [], depth: 0, totalCount: nb.noteCount });
  const roots: NotebookNode[] = [];
  for (const node of byId.values()) {
    const parent = node.parentId != null ? byId.get(node.parentId) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  const visit = (nodes: NotebookNode[], depth: number): number => {
    let sum = 0;
    for (const n of nodes) {
      n.depth = depth;
      n.totalCount = n.noteCount + visit(n.children, depth + 1);
      sum += n.totalCount;
    }
    return sum;
  };
  visit(roots, 0);
  return roots;
}

/** Depth-first list with indentation, e.g. for "move to" menus. */
export function flattenTree(nodes: NotebookNode[]): NotebookNode[] {
  const out: NotebookNode[] = [];
  const walk = (ns: NotebookNode[]) => {
    for (const n of ns) {
      out.push(n);
      walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

/** "父 / 子" path of a notebook. */
export function notebookPath(list: Notebook[], id: number | null | undefined): string {
  if (id == null) return "";
  const byId = new Map(list.map((n) => [n.id, n]));
  const parts: string[] = [];
  let cur = byId.get(id);
  let guard = 0;
  while (cur && guard++ < 50) {
    parts.unshift(cur.name);
    cur = cur.parentId != null ? byId.get(cur.parentId) : undefined;
  }
  return parts.join(" / ");
}

export const DND_NOTE = "application/x-blog-note";
export const DND_NOTEBOOK = "application/x-blog-notebook";
