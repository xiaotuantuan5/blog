-- 第一期：笔记、笔记本、标签、附件、设置、管理员登录

CREATE TABLE admin (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at    TEXT NOT NULL
);

CREATE TABLE sessions (
  token_hash  TEXT PRIMARY KEY,
  admin_id    INTEGER NOT NULL REFERENCES admin(id) ON DELETE CASCADE,
  csrf_token  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE notebooks (
  id         INTEGER PRIMARY KEY,
  parent_id  INTEGER REFERENCES notebooks(id) ON DELETE RESTRICT,
  name       TEXT NOT NULL,
  sort       INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE INDEX notebooks_parent ON notebooks(parent_id);

CREATE TABLE notes (
  id           INTEGER PRIMARY KEY,
  notebook_id  INTEGER REFERENCES notebooks(id) ON DELETE RESTRICT,
  title        TEXT NOT NULL,
  slug         TEXT NOT NULL UNIQUE,
  content_md   TEXT NOT NULL DEFAULT '',
  content_html TEXT NOT NULL DEFAULT '',
  content_text TEXT NOT NULL DEFAULT '',   -- 纯文本，用于搜索和摘要
  toc_json     TEXT NOT NULL DEFAULT '[]',
  summary      TEXT NOT NULL DEFAULT '',   -- 手填摘要，为空时自动截取正文
  visibility   TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
  pinned       INTEGER NOT NULL DEFAULT 0,
  favorite     INTEGER NOT NULL DEFAULT 0,
  version      INTEGER NOT NULL DEFAULT 1, -- 乐观锁，防止多个标签页互相覆盖
  published_at TEXT,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX notes_notebook ON notes(notebook_id);
CREATE INDEX notes_updated ON notes(updated_at);
CREATE INDEX notes_public ON notes(visibility, published_at);

CREATE TABLE tags (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE
);

CREATE TABLE note_tags (
  note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (note_id, tag_id)
);
CREATE INDEX note_tags_tag ON note_tags(tag_id);

CREATE TABLE files (
  id         INTEGER PRIMARY KEY,
  hash       TEXT NOT NULL UNIQUE,   -- sha256，同时是存储文件名
  filename   TEXT NOT NULL,
  mime       TEXT NOT NULL,
  size       INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE note_files (
  note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  PRIMARY KEY (note_id, file_id)
);
CREATE INDEX note_files_file ON note_files(file_id);

-- 全文索引（trigram 分词支持中文子串搜索）
CREATE VIRTUAL TABLE notes_fts USING fts5(
  title, content_text,
  content = 'notes', content_rowid = 'id',
  tokenize = 'trigram'
);

CREATE TRIGGER notes_ai AFTER INSERT ON notes BEGIN
  INSERT INTO notes_fts (rowid, title, content_text) VALUES (new.id, new.title, new.content_text);
END;
CREATE TRIGGER notes_ad AFTER DELETE ON notes BEGIN
  INSERT INTO notes_fts (notes_fts, rowid, title, content_text) VALUES ('delete', old.id, old.title, old.content_text);
END;
CREATE TRIGGER notes_au AFTER UPDATE OF title, content_text ON notes BEGIN
  INSERT INTO notes_fts (notes_fts, rowid, title, content_text) VALUES ('delete', old.id, old.title, old.content_text);
  INSERT INTO notes_fts (rowid, title, content_text) VALUES (new.id, new.title, new.content_text);
END;
