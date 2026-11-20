export type Visibility = "private" | "public";

export interface Heading {
  level: number;
  id: string;
  text: string;
}

export interface NoteSummary {
  id: number;
  notebookId: number | null;
  title: string;
  slug: string;
  summary: string;
  visibility: Visibility;
  pinned: boolean;
  favorite: boolean;
  tags: string[];
  version: number;
  publishedAt: string | null;
  createdAt: string;
  updatedAt: string;
  /** HTML-escaped excerpt with <mark> around matches (search only). */
  snippet?: string;
}

export interface Note extends NoteSummary {
  manualSummary: string;
  contentMd: string;
  contentHtml: string;
  toc: Heading[];
}

export interface NoteLink {
  title: string;
  slug: string;
}

export interface PublicNote extends Omit<Note, "contentMd" | "manualSummary"> {
  prev: NoteLink | null;
  next: NoteLink | null;
}

export interface NotePage {
  items: NoteSummary[];
  total: number;
  page: number;
  pageSize: number;
}

export interface NoteInput {
  title: string;
  slug: string;
  notebookId?: number | null;
  contentMd: string;
  summary: string;
  visibility: Visibility;
  tags: string[];
  version?: number;
}

export interface NoteFlags {
  pinned?: boolean;
  favorite?: boolean;
  notebookId?: number | null;
}

export interface Notebook {
  id: number;
  parentId: number | null;
  name: string;
  sort: number;
  noteCount: number;
  createdAt: string;
}

export interface Tag {
  id: number;
  name: string;
  count: number;
}

export interface ArchiveItem {
  title: string;
  slug: string;
  publishedAt: string;
}

export interface Site {
  siteTitle: string;
  siteDescription: string;
  author: string;
  avatar: string;
  aboutHtml: string;
}

export interface Settings extends Site {
  siteUrl: string;
  aboutMd: string;
  pageSize: number;
}

export interface Me {
  loggedIn: boolean;
  username?: string;
  csrfToken?: string;
}

export interface Stats {
  notes: number;
  public: number;
  notebooks: number;
  tags: number;
  files: number;
}

export interface UploadedFile {
  id: number;
  hash: string;
  filename: string;
  mime: string;
  size: number;
  url: string;
  isImage: boolean;
}

export interface Preview {
  html: string;
  toc: Heading[];
}
