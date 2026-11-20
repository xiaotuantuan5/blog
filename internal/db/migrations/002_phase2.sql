-- 第二期：知识管理（双链、历史版本、回收站、每日笔记、学习热力图、划线批注）

ALTER TABLE notes ADD COLUMN deleted_at TEXT;
ALTER TABLE notes ADD COLUMN is_daily INTEGER NOT NULL DEFAULT 0;

CREATE INDEX notes_deleted ON notes(deleted_at);

-- 双链：保存笔记时解析 [[标题]] 存表，反向链接按 target 查询
CREATE TABLE note_links (
  source_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  target_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  PRIMARY KEY (source_id, target_id)
);
CREATE INDEX note_links_target ON note_links(target_id);

-- 历史版本：保存前快照旧内容
CREATE TABLE note_revisions (
  id         INTEGER PRIMARY KEY,
  note_id    INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  title      TEXT NOT NULL,
  content_md TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX note_revisions_note ON note_revisions(note_id, id);

-- 学习热力图：一天一篇笔记一个动作只记一行
CREATE TABLE activity (
  day     TEXT NOT NULL,
  note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  action  TEXT NOT NULL CHECK (action IN ('create', 'edit')),
  PRIMARY KEY (day, note_id, action)
);

-- 划线批注：位置按 W3C Web Annotation 的文字锚点保存
CREATE TABLE annotations (
  id           INTEGER PRIMARY KEY,
  note_id      INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  exact        TEXT NOT NULL,
  prefix       TEXT NOT NULL DEFAULT '',
  suffix       TEXT NOT NULL DEFAULT '',
  start_offset INTEGER NOT NULL DEFAULT 0,
  end_offset   INTEGER NOT NULL DEFAULT 0,
  color        TEXT NOT NULL DEFAULT 'yellow' CHECK (color IN ('yellow', 'green', 'blue', 'red')),
  comment_md   TEXT NOT NULL DEFAULT '',
  comment_html TEXT NOT NULL DEFAULT '',
  comment_text TEXT NOT NULL DEFAULT '',   -- 批注纯文本，供搜索
  orphaned     INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL
);
CREATE INDEX annotations_note ON annotations(note_id);

CREATE VIRTUAL TABLE annotations_fts USING fts5(
  exact, comment_text,
  content = 'annotations', content_rowid = 'id',
  tokenize = 'trigram'
);
CREATE TRIGGER annotations_ai AFTER INSERT ON annotations BEGIN
  INSERT INTO annotations_fts (rowid, exact, comment_text) VALUES (new.id, new.exact, new.comment_text);
END;
CREATE TRIGGER annotations_ad AFTER DELETE ON annotations BEGIN
  INSERT INTO annotations_fts (annotations_fts, rowid, exact, comment_text) VALUES ('delete', old.id, old.exact, old.comment_text);
END;
CREATE TRIGGER annotations_au AFTER UPDATE OF exact, comment_md ON annotations BEGIN
  INSERT INTO annotations_fts (annotations_fts, rowid, exact, comment_text) VALUES ('delete', old.id, old.exact, old.comment_text);
  INSERT INTO annotations_fts (rowid, exact, comment_text) VALUES (new.id, new.exact, new.comment_text);
END;
