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
  /** Set while the note sits in the trash. */
  deletedAt: string | null;
  /** HTML-escaped excerpt with <mark> around matches (search only). */
  snippet?: string;
}

export interface Backlink {
  id: number;
  title: string;
  visibility: Visibility;
}

export interface Note extends NoteSummary {
  manualSummary: string;
  contentMd: string;
  contentHtml: string;
  toc: Heading[];
  backlinks: Backlink[] | null;
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
  /** Manual saves always snapshot a revision. */
  manual?: boolean;
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
  trash: number;
}

export interface TrashItem extends NoteSummary {
  notebookName: string;
}

export interface Revision {
  id: number;
  noteId: number;
  title: string;
  contentMd: string;
  createdAt: string;
}

export type AnnotationColor = "yellow" | "green" | "blue" | "red";

export interface Annotation {
  id: number;
  noteId: number;
  exact: string;
  prefix: string;
  suffix: string;
  startOffset: number;
  endOffset: number;
  color: AnnotationColor;
  commentMd: string;
  commentHtml: string;
  orphaned: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AnnotationListItem extends Annotation {
  noteTitle: string;
  notebookId: number | null;
  noteUpdatedAt: string;
}

export interface AnnotationPage {
  items: AnnotationListItem[];
  total: number;
  page: number;
  pageSize: number;
}

export interface AnnotationInput {
  exact: string;
  prefix: string;
  suffix: string;
  startOffset: number;
  endOffset: number;
  color: AnnotationColor;
  commentMd: string;
  orphaned?: boolean;
}

export interface DayCount {
  day: string;
  count: number;
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
