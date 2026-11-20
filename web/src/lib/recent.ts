interface RecentEntry {
  id: number;
  title: string;
  updatedAt: string;
}

const KEY = "recent-notes";
const MAX = 8;

/** Locally tracked recently opened notes for the sidebar. */
export function getRecent(): RecentEntry[] {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const list = JSON.parse(raw) as RecentEntry[];
    return Array.isArray(list) ? list.filter((n) => n && typeof n.id === "number").slice(0, MAX) : [];
  } catch {
    return [];
  }
}

/** Stores only list fields — never the note body. */
export function pushRecent(note: { id: number; title: string; updatedAt: string }) {
  const entry: RecentEntry = { id: note.id, title: note.title, updatedAt: note.updatedAt };
  const list = getRecent().filter((n) => n.id !== entry.id);
  list.unshift(entry);
  try {
    localStorage.setItem(KEY, JSON.stringify(list.slice(0, MAX)));
  } catch {
    /* storage full */
  }
  window.dispatchEvent(new Event("recent-notes"));
}
