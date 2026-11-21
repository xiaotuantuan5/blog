-- HTML 笔记：内容格式标记，md = Markdown（默认），html = 直接书写 HTML

ALTER TABLE notes ADD COLUMN format TEXT NOT NULL DEFAULT 'md';
ALTER TABLE note_revisions ADD COLUMN format TEXT NOT NULL DEFAULT 'md';
