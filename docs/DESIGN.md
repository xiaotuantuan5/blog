# 个人笔记博客系统 设计文档

## 1. 定位

一个自己用的知识库，兼做博客。

- 默认所有笔记**私有**，只有登录后能看到。
- 单篇笔记可以设为**公开**，公开的笔记组成对外的博客。
- 单用户（只有你一个管理员），不做评论、不做多人协作。
- 一个二进制文件 + 一个数据目录即可运行，前端不加载任何外部资源（无 CDN、无第三方字体和统计）。

## 2. 架构

```
┌──────────── blog（单个二进制）────────────┐
│  net/http 路由                             │
│   ├── /api/*         JSON API              │
│   ├── /files/*       附件和图片（鉴权）     │
│   ├── /rss.xml /sitemap.xml（仅公开笔记）   │
│   └── /*             React SPA（go:embed） │
│  SQLite（modernc.org/sqlite，纯 Go，无 CGO）│
│  后台任务：自动备份、回收站清理              │
└────────────────────────────────────────────┘
        data/
        ├── blog.db
        ├── files/        上传的图片和附件
        └── backups/      自动备份
```

构建：`make build` 先执行 `vite build`，再执行 `CGO_ENABLED=0 go build`，产出 `bin/blog`。
交叉编译：`GOOS=linux GOARCH=amd64 make build`。

启动：

```
./blog init-admin -u admin          # 首次创建管理员，交互输入密码
./blog serve -addr :8080 -data ./data
```

## 3. 技术选型

### 后端（Go 1.22+）

| 用途 | 选择 | 说明 |
|---|---|---|
| HTTP | 标准库 `net/http` | 1.22 的 ServeMux 已支持方法匹配和路径参数 |
| 数据库 | SQLite + `modernc.org/sqlite` | 纯 Go，可交叉编译；开启 WAL 模式 |
| 迁移 | `migrations/*.sql` 用 embed 打包，启动时按版本执行 | 升级只需替换二进制 |
| Markdown | `goldmark` + GFM、脚注扩展，自定义 `[[双链]]` 扩展 | 后端是唯一的渲染来源，预览和最终效果一致 |
| 代码高亮 | `chroma`（输出 CSS class） | 高亮在服务端完成，前端不需要高亮库 |
| HTML 过滤 | `bluemonday` | 防止粘贴进来的 HTML 带脚本 |
| 搜索 | SQLite FTS5 + `trigram` 分词器 | 支持中文子串搜索；少于 3 个字时退回 `LIKE` |
| 密码 | `bcrypt` | |
| 会话 | 随机 token，存 SHA-256 哈希，HttpOnly + SameSite=Lax Cookie | 写操作额外校验 CSRF 头 |
| 日志 | 标准库 `log/slog` | |

### 前端（React SPA）

| 用途 | 选择 |
|---|---|
| 框架 | React 19 + TypeScript + Vite |
| 路由 | React Router |
| 请求和缓存 | TanStack Query |
| 本地状态 | Zustand（侧栏、主题、编辑器设置） |
| 样式 | Tailwind CSS（构建时生成，无运行时依赖） |
| 组件 | shadcn/ui（源码拷进项目，基于 Radix） |
| 编辑器 | CodeMirror 6（Markdown 模式、Vim 键位可选） |
| 命令面板 | cmdk |
| 数学公式 | KaTeX（字体随产物打包） |
| 图表 | Mermaid（按需懒加载，不影响首屏） |
| 图标 | lucide-react（打包为 SVG） |

## 4. 功能

按优先级分三期，每期都能独立使用。

### 第一期：能用的笔记 + 博客

**笔记**
- 新建、编辑、删除笔记，Markdown 格式
- 笔记本：树形目录，可拖拽排序和移动
- 标签：多标签，标签页聚合
- 状态：私有 / 公开；可置顶、可收藏
- 自动保存：停止输入 1 秒后保存，离开页面前未保存会提示
- 编辑器：左写右预览 / 只写 / 只看 三种模式；常用快捷键（加粗、链接、代码块）

**Markdown 支持**
- GFM：表格、任务列表、删除线、自动链接
- 代码高亮 + 一键复制 + 显示语言
- 数学公式 `$...$` / `$$...$$`
- Mermaid 流程图、时序图
- 提示块：`> [!NOTE]`、`> [!TIP]`、`> [!WARNING]`
- 脚注、自动生成目录

**附件**
- 粘贴或拖拽图片直接上传，自动插入 Markdown
- 支持 PDF 等附件，限制类型和大小
- 文件按内容哈希命名，相同文件只存一份

**搜索**
- 全文搜索标题和正文，结果高亮关键词
- 可按笔记本、标签、状态过滤

**公开博客**
- 首页列出公开笔记，有标签页、归档页、关于页
- 访问 `/n/:slug` 时，后端向 `index.html` 注入标题、摘要和 Open Graph 标签，方便搜索引擎和分享
- RSS、sitemap 只包含公开笔记

**基础**
- 登录 / 退出，登录失败限流
- 暗色模式，移动端可阅读
- 站点设置：站点名、简介、头像

### 第二期：知识管理

- **双向链接**：正文写 `[[笔记标题]]` 链接到其他笔记；保存时解析链接存表，每篇笔记底部显示"反向链接"（哪些笔记引用了它）。公开笔记链接到私有笔记时，对访客只显示纯文本
- **命令面板**：`Ctrl/Cmd + K` 快速搜索并跳转笔记、执行命令（新建笔记、切换主题等）
- **历史版本**：每次手动保存或自动保存间隔超过 10 分钟时记录一个版本；可以查看差异、恢复到某个版本
- **回收站**：删除先进回收站，30 天后自动清理，可恢复
- **每日笔记**：一键打开今天的日记（`日记/2026-11-20`），适合记学习日志
- **学习热力图**：类似 GitHub 贡献图，按天统计新建和编辑的笔记数
- **最近访问**：侧栏显示最近打开的笔记
- **划线批注**：在阅读视图里直接对文章做标注，不改动原文，详见第 4.1 节

### 4.1 划线批注

**使用方式**
- 在阅读视图中选中一段文字，弹出浮动工具栏：
  - 4 种颜色高亮（黄 / 绿 / 蓝 / 红，可以自己约定含义，例如"重点 / 疑问 / 待查 / 错误"）
  - 添加批注（Markdown，支持代码和公式）
  - 复制带出处的引用
  - 生成闪卡（选中文字作为答案，自己填问题，第三期上线后可用）
- 高亮部分鼠标悬停时显示批注，点击可编辑或删除
- 右侧面板按原文顺序列出本篇所有批注，点击跳到对应位置；可按颜色筛选
- 一键显示 / 隐藏所有高亮，方便干净地重读
- 批注**只有自己可见**：访客看公开文章时不会加载批注接口，也看不到高亮

**批注汇总页 `/app/annotations`**
- 列出所有笔记的批注，可按颜色、标签、笔记本筛选，可搜索批注内容
- 适合复习时只看"疑问"类批注，或回顾某个主题下的重点

**原文修改后批注如何保持位置**

批注不保存 DOM 路径（原文一改就失效），而是参考 W3C Web Annotation 的做法，保存文字本身：

- `exact`：选中的原文
- `prefix` / `suffix`：前后各 32 个字符的上下文
- `start` / `end`：在全文纯文本中的字符位置（只作参考）

前端渲染文章后，按以下顺序重新定位：

1. 在纯文本中查找 `exact`，有多处时选前后文匹配、位置最接近的一处
2. 找不到时做模糊匹配（`diff-match-patch` 的 match 算法，随前端打包），允许少量改动
3. 仍然找不到就标记为"失效"，在右侧面板单独列出并显示当时的原文，可以手动删除或重新选择位置

定位到之后，用 `TreeWalker` 遍历文本节点，把选区包进 `<mark>`，跨段落、跨加粗等格式的选区也能正确高亮。代码块和公式内部可以高亮，但按整块处理。

**导出**：导出 zip 时，批注写入每篇笔记的同名 `.annotations.md` 文件（引用原文 + 批注内容），不改动笔记正文。

### 第三期：学习工具

- **闪卡复习**：在笔记里用固定语法写问答卡片，例如

  ```
  ?? 什么是 goroutine 泄漏
  :: goroutine 一直阻塞，无法退出，占用的内存无法回收
  ```

  保存时提取为卡片，用 SM-2 间隔重复算法安排复习。复习页按"忘了 / 模糊 / 记得"打分，自动计算下次复习日期。首页显示今天待复习的数量。
- **学习专题**：把多篇笔记组织成有序的专题（例如"Go 并发"），每篇标记"未学 / 学习中 / 已掌握"，显示专题进度
- **待办汇总**：汇总所有笔记中未完成的 `- [ ]` 任务，点击跳回原笔记
- **导入导出**：
  - 导出全部笔记为 zip（Markdown 文件 + 附件，按笔记本分目录，frontmatter 里保留标签和状态）
  - 导入 Markdown 文件夹（兼容 Obsidian 的 `[[双链]]` 和图片引用）
- **自动备份**：每天用 `VACUUM INTO` 备份数据库到 `data/backups/`，保留最近 N 份；后台可以手动下载备份

## 5. 数据模型

```sql
-- 笔记
notes (
  id            INTEGER PRIMARY KEY,
  notebook_id   INTEGER REFERENCES notebooks(id),
  title         TEXT NOT NULL,
  slug          TEXT UNIQUE NOT NULL,       -- 公开访问地址
  content_md    TEXT NOT NULL DEFAULT '',
  content_html  TEXT NOT NULL DEFAULT '',   -- 保存时渲染好
  summary       TEXT NOT NULL DEFAULT '',   -- 手填或自动截取
  visibility    TEXT NOT NULL DEFAULT 'private',  -- private | public
  pinned        INTEGER NOT NULL DEFAULT 0,
  favorite      INTEGER NOT NULL DEFAULT 0,
  is_daily      INTEGER NOT NULL DEFAULT 0,
  published_at  DATETIME,
  created_at    DATETIME NOT NULL,
  updated_at    DATETIME NOT NULL,
  deleted_at    DATETIME                    -- 非空表示在回收站
)

notebooks (id, parent_id, name, sort, created_at)
tags (id, name UNIQUE)
note_tags (note_id, tag_id, PRIMARY KEY(note_id, tag_id))

note_links (source_id, target_id, PRIMARY KEY(source_id, target_id))  -- 双链
note_revisions (id, note_id, title, content_md, created_at)            -- 历史版本

files (id, hash UNIQUE, filename, mime, size, created_at)
note_files (note_id, file_id)

-- 划线批注
annotations (
  id            INTEGER PRIMARY KEY,
  note_id       INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  exact         TEXT NOT NULL,              -- 选中的原文
  prefix        TEXT NOT NULL DEFAULT '',
  suffix        TEXT NOT NULL DEFAULT '',
  start_offset  INTEGER NOT NULL,           -- 纯文本中的位置，仅作定位参考
  end_offset    INTEGER NOT NULL,
  color         TEXT NOT NULL DEFAULT 'yellow',  -- yellow | green | blue | red
  comment_md    TEXT NOT NULL DEFAULT '',
  comment_html  TEXT NOT NULL DEFAULT '',
  orphaned      INTEGER NOT NULL DEFAULT 0, -- 原文修改后无法定位
  created_at    DATETIME NOT NULL,
  updated_at    DATETIME NOT NULL
)

-- 闪卡
cards (
  id, note_id, question, answer,
  ease REAL DEFAULT 2.5, interval_days INTEGER DEFAULT 0,
  repetitions INTEGER DEFAULT 0, due_date DATE, updated_at
)
card_reviews (id, card_id, grade, reviewed_at)

-- 学习专题
topics (id, title, description, sort, created_at)
topic_notes (topic_id, note_id, sort, status)  -- status: todo | learning | done

-- 统计
activity (day DATE, note_id, action)  -- action: create | edit，用于热力图

-- 系统
settings (key PRIMARY KEY, value)
admin (id, username UNIQUE, password_hash)
sessions (token_hash PRIMARY KEY, admin_id, expires_at, created_at)

-- 全文索引，用触发器与 notes 同步
CREATE VIRTUAL TABLE notes_fts USING fts5(
  title, content_md, content='notes', content_rowid='id', tokenize='trigram'
);
CREATE VIRTUAL TABLE annotations_fts USING fts5(
  exact, comment_md, content='annotations', content_rowid='id', tokenize='trigram'
);
```

## 6. API

所有接口返回 JSON。`/api/admin/*` 需要登录，写操作需要 `X-CSRF-Token` 头。

### 公开接口（只返回 `visibility = public` 且未删除的笔记）

```
GET  /api/site                         站点信息
GET  /api/public/notes?page=&tag=&q=   公开笔记列表 / 搜索
GET  /api/public/notes/:slug           公开笔记详情
GET  /api/public/tags
GET  /api/public/archive
```

### 管理接口

```
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/auth/me

GET    /api/admin/notes?notebook=&tag=&q=&visibility=&page=
POST   /api/admin/notes
GET    /api/admin/notes/:id                   含反向链接
PUT    /api/admin/notes/:id
DELETE /api/admin/notes/:id                   移入回收站
POST   /api/admin/notes/:id/restore
DELETE /api/admin/notes/:id/purge             彻底删除
GET    /api/admin/notes/:id/revisions
POST   /api/admin/notes/:id/revisions/:rid/restore
GET    /api/admin/daily/:date                 获取或创建某天的日记
POST   /api/admin/preview                     Markdown 预览

CRUD   /api/admin/notebooks
GET    /api/admin/tags     PUT/DELETE /api/admin/tags/:id
POST   /api/admin/files                       上传
GET    /api/admin/trash
GET    /api/admin/stats/heatmap?year=
GET    /api/admin/tasks                       待办汇总

GET    /api/admin/notes/:id/annotations       本篇批注
POST   /api/admin/notes/:id/annotations       新建批注
PUT    /api/admin/annotations/:id             改颜色、改批注、重新定位
DELETE /api/admin/annotations/:id
GET    /api/admin/annotations?color=&tag=&notebook=&q=   批注汇总

GET    /api/admin/cards/due                   今日待复习
POST   /api/admin/cards/:id/review            {grade: 0-5}
CRUD   /api/admin/topics
PUT    /api/admin/topics/:id/notes/:noteId    更新学习状态

GET    /api/admin/export                      下载 zip
POST   /api/admin/import                      上传 zip
GET    /api/admin/backups   GET /api/admin/backups/:name
GET/PUT /api/admin/settings
```

### 其他

```
GET /files/:hash          附件；只被公开笔记引用的文件可匿名访问
GET /rss.xml
GET /sitemap.xml
GET /*                    SPA；/n/:slug 会注入 SEO 元信息
```

## 7. 前端页面

```
公开区
/                     首页（公开笔记列表）
/n/:slug              笔记阅读页（目录、上一篇/下一篇）
/tags/:name           标签
/archive              归档
/about                关于

工作区（登录后）
/login
/app                  工作台：今日待复习、最近编辑、热力图、待办数
/app/notes            笔记列表（左侧笔记本树 + 标签）
/app/notes/:id        阅读视图（划线批注、目录、反向链接），可切换到编辑器（预览、历史版本）
/app/annotations      批注汇总
/app/daily            今天的日记
/app/search           搜索
/app/review           闪卡复习
/app/topics           学习专题
/app/tasks            待办汇总
/app/trash            回收站
/app/settings         站点设置、导入导出、备份
```

工作区布局：左侧是可折叠的侧栏（笔记本树、标签、快捷入口），中间是笔记列表或编辑器，右侧是可折叠的目录和反向链接面板。

## 8. 目录结构

```
blog/
├── cmd/blog/main.go           入口，子命令 serve / init-admin / reset-password
├── internal/
│   ├── config/                命令行参数和环境变量
│   ├── db/
│   │   ├── db.go              连接、PRAGMA、迁移
│   │   └── migrations/*.sql
│   ├── store/                 数据访问（每张表一个文件）
│   ├── service/               业务逻辑
│   │   ├── note.go            保存时：渲染、解析双链和闪卡、写版本、记活动
│   │   ├── search.go
│   │   ├── srs.go             SM-2 算法
│   │   ├── exportimport.go
│   │   └── backup.go
│   ├── markdown/              goldmark 配置和自定义扩展（双链、提示块、闪卡）
│   ├── handler/               HTTP handler
│   ├── middleware/            鉴权、CSRF、限流、日志、recover、安全响应头
│   └── server/                路由注册、SPA 回退、SEO 注入
├── web/
│   ├── src/
│   │   ├── api/               请求封装和类型
│   │   ├── components/
│   │   ├── features/          notes / editor / review / topics ...
│   │   ├── layouts/           PublicLayout、WorkspaceLayout
│   │   ├── routes/
│   │   └── stores/
│   ├── index.html
│   ├── vite.config.ts
│   └── embed.go               //go:embed all:dist
├── docs/DESIGN.md
├── Makefile
└── README.md
```

## 9. 安全

- 所有 SQL 使用参数化查询
- Markdown 渲染结果经过 bluemonday 过滤
- 安全响应头：`Content-Security-Policy: default-src 'self'`（前端无外部资源，可以设得很严）、`X-Content-Type-Options: nosniff`、`Referrer-Policy`
- 上传：检查文件实际类型（不信任扩展名），限制大小，SVG 以附件形式下载，不内联显示
- 登录失败按 IP 限流，连续失败后延迟响应
- 私有笔记和附件在每个接口都校验登录状态，公开接口在 SQL 层过滤 `visibility = 'public'`

## 10. 开发方式

- 开发时：`go run ./cmd/blog serve -dev` 启动后端（8080），`cd web && npm run dev` 启动 Vite（5173），Vite 把 `/api` 代理到 8080
- 构建时：`make build` 生成单个二进制
- 测试：Go 部分写 `store`、`service`、`markdown` 的单元测试；前端以类型检查和构建通过为准
