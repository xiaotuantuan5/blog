# blog

个人笔记博客系统：Go + SQLite + React，打包为单个二进制，前端不依赖任何外部资源。

- 笔记默认私有，单篇可设为公开，公开的笔记组成博客
- Markdown：代码高亮、数学公式、Mermaid 图表、提示块、任务列表、脚注
- 粘贴或拖入图片直接上传；中文全文搜索；自动保存 + 本地草稿 + 版本冲突检测

设计文档见 [docs/DESIGN.md](docs/DESIGN.md)。

## 构建

需要 Go 1.25+ 和 Node.js 22+。

```bash
make build                         # 生成 bin/blog
GOOS=linux GOARCH=arm64 make build # 交叉编译
```

## 运行

```bash
./bin/blog init-admin -u admin -data ./data   # 首次运行，按提示输入密码（至少 8 位）
./bin/blog serve -addr :8080 -data ./data
```

然后访问 `http://服务器:8080`，点页脚的“登录”进入工作台。

| 参数 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `-addr` | `BLOG_ADDR` | `:8080` | 监听地址 |
| `-data` | `BLOG_DATA` | `./data` | 数据目录：`blog.db` 和 `files/` |
| `-max-upload-mb` | `BLOG_MAX_UPLOAD_MB` | `20` | 单个附件大小上限 |
| `-trust-proxy` | `BLOG_TRUST_PROXY` | `false` | 在 Nginx/Caddy 后面时开启，用于获取真实 IP 和 HTTPS 状态 |

其他命令：

```bash
./bin/blog reset-password -u admin -data ./data   # 忘记密码时重置（会退出所有登录）
BLOG_ADMIN_PASSWORD=xxx ./bin/blog init-admin     # 非交互方式
```

**备份**：停止服务后复制整个 `data/` 目录即可。第三期会加入在线自动备份。

## 开发

```bash
make dev-api   # 终端 1：后端 http://127.0.0.1:8080
make dev-web   # 终端 2：前端 http://localhost:5173（代理 /api 到后端）
make check     # 后端测试 + vet + 前端类型检查
```

修改代码高亮主题后运行 `make chroma-css` 重新生成 `web/src/styles/chroma.css`。

## 目录

```
cmd/blog/            入口和子命令
internal/db/         SQLite 连接和迁移（migrations/*.sql）
internal/core/       业务逻辑和 SQL
internal/markdown/   Markdown 渲染（goldmark + chroma + bluemonday）
internal/server/     HTTP API、附件、RSS、SPA 和 SEO 注入
web/                 React 前端，构建产物通过 go:embed 打进二进制
tools/chromacss/     生成代码高亮样式
```
