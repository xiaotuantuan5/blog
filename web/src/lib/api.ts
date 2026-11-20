import type {
  Annotation,
  AnnotationInput,
  AnnotationPage,
  ArchiveItem,
  DayCount,
  Me,
  Note,
  NoteFlags,
  NoteInput,
  NotePage,
  Notebook,
  Preview,
  PublicNote,
  Revision,
  Settings,
  Site,
  Stats,
  Tag,
  TrashItem,
  UploadedFile,
} from "./types";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

let csrfToken = "";

export function setCsrfToken(token: string | undefined) {
  csrfToken = token ?? "";
}

/** Fired when an admin request comes back 401 (session expired). */
export const AUTH_EXPIRED_EVENT = "auth:expired";

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  let payload: BodyInit | undefined;
  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  if (method !== "GET") headers["X-CSRF-Token"] = csrfToken;

  let res: Response;
  try {
    res = await fetch(url, { method, headers, body: payload, credentials: "same-origin" });
  } catch {
    throw new ApiError(0, "网络错误，请检查网络连接");
  }
  if (!res.ok) {
    let msg = `请求失败（${res.status}）`;
    try {
      const data = await res.json();
      if (data?.error) msg = data.error;
    } catch {
      /* not JSON */
    }
    if (res.status === 401 && (url.startsWith("/api/admin") || url === "/api/auth/logout")) {
      window.dispatchEvent(new Event(AUTH_EXPIRED_EVENT));
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function qs(params: Record<string, string | number | boolean | null | undefined>) {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === "" || v === false) continue;
    sp.set(k, v === true ? "1" : String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : "";
}

export interface AdminNoteQuery {
  notebook?: number | null;
  tag?: string;
  q?: string;
  visibility?: string;
  favorite?: boolean;
  page?: number;
  pageSize?: number;
}

export const api = {
  // Public
  site: () => request<Site>("GET", "/api/site"),
  publicNotes: (p: { page?: number; tag?: string; q?: string }) =>
    request<NotePage>("GET", "/api/public/notes" + qs(p)),
  publicNote: (slug: string) => request<PublicNote>("GET", `/api/public/notes/${encodeURIComponent(slug)}`),
  publicTags: () => request<Tag[]>("GET", "/api/public/tags"),
  archive: () => request<ArchiveItem[]>("GET", "/api/public/archive"),

  // Auth
  me: () => request<Me>("GET", "/api/auth/me"),
  login: (username: string, password: string) => request<Me>("POST", "/api/auth/login", { username, password }),
  logout: () => request<Me>("POST", "/api/auth/logout"),
  changePassword: (oldPassword: string, newPassword: string) =>
    request<void>("PUT", "/api/auth/password", { oldPassword, newPassword }),

  // Notes
  notes: (q: AdminNoteQuery) => request<NotePage>("GET", "/api/admin/notes" + qs({ ...q })),
  note: (id: number) => request<Note>("GET", `/api/admin/notes/${id}`),
  createNote: (input: NoteInput) => request<Note>("POST", "/api/admin/notes", input),
  updateNote: (id: number, input: NoteInput) => request<Note>("PUT", `/api/admin/notes/${id}`, input),
  patchNote: (id: number, flags: NoteFlags) => request<void>("PATCH", `/api/admin/notes/${id}`, flags),
  deleteNote: (id: number) => request<void>("DELETE", `/api/admin/notes/${id}`),
  restoreNote: (id: number) => request<void>("POST", `/api/admin/notes/${id}/restore`),
  purgeNote: (id: number) => request<void>("DELETE", `/api/admin/notes/${id}/purge`),
  trash: () => request<TrashItem[]>("GET", "/api/admin/trash"),
  revisions: (id: number) => request<Revision[]>("GET", `/api/admin/notes/${id}/revisions`),
  restoreRevision: (id: number, rid: number) =>
    request<Note>("POST", `/api/admin/notes/${id}/revisions/${rid}/restore`),
  daily: (date: string) => request<Note>("GET", `/api/admin/daily/${date}`),
  heatmap: (from: string, to: string) =>
    request<DayCount[]>("GET", "/api/admin/stats/heatmap" + qs({ from, to })),
  preview: (contentMd: string) => request<Preview>("POST", "/api/admin/preview", { contentMd }),

  // Annotations
  annotations: (noteId: number) => request<Annotation[]>("GET", `/api/admin/notes/${noteId}/annotations`),
  createAnnotation: (noteId: number, input: AnnotationInput) =>
    request<Annotation>("POST", `/api/admin/notes/${noteId}/annotations`, input),
  updateAnnotation: (id: number, input: AnnotationInput) =>
    request<Annotation>("PUT", `/api/admin/annotations/${id}`, input),
  deleteAnnotation: (id: number) => request<void>("DELETE", `/api/admin/annotations/${id}`),
  annotationList: (p: { color?: string; notebook?: number; q?: string; page?: number }) =>
    request<AnnotationPage>("GET", "/api/admin/annotations" + qs(p)),

  // Notebooks & tags
  notebooks: () => request<Notebook[]>("GET", "/api/admin/notebooks"),
  createNotebook: (name: string, parentId: number | null) =>
    request<Notebook>("POST", "/api/admin/notebooks", { name, parentId }),
  updateNotebook: (id: number, input: { name: string; parentId: number | null; sort: number }) =>
    request<Notebook>("PUT", `/api/admin/notebooks/${id}`, input),
  deleteNotebook: (id: number) => request<void>("DELETE", `/api/admin/notebooks/${id}`),
  tags: () => request<Tag[]>("GET", "/api/admin/tags"),
  renameTag: (id: number, name: string) => request<void>("PUT", `/api/admin/tags/${id}`, { name }),
  deleteTag: (id: number) => request<void>("DELETE", `/api/admin/tags/${id}`),

  // Files, settings, stats
  upload: (file: File) => {
    const fd = new FormData();
    fd.append("file", file, file.name);
    return request<UploadedFile>("POST", "/api/admin/files", fd);
  },
  settings: () => request<Settings>("GET", "/api/admin/settings"),
  updateSettings: (s: Settings) => request<Settings>("PUT", "/api/admin/settings", s),
  stats: () => request<Stats>("GET", "/api/admin/stats"),
};
