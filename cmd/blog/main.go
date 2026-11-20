// Command blog runs the personal notes and blog server.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/xiaotuantuan5/blog/internal/core"
	"github.com/xiaotuantuan5/blog/internal/db"
	"github.com/xiaotuantuan5/blog/internal/server"
	"github.com/xiaotuantuan5/blog/web"
)

const usage = `用法：blog <命令> [参数]

命令：
  serve            启动服务
  init-admin       创建管理员账号（首次使用）
  reset-password   重置管理员密码

运行 blog <命令> -h 查看参数。参数也可以通过环境变量设置：
  BLOG_ADDR  BLOG_DATA  BLOG_MAX_UPLOAD_MB  BLOG_TRUST_PROXY  BLOG_ADMIN_PASSWORD
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "init-admin":
		err = adminCmd("init-admin", os.Args[2:])
	case "reset-password":
		err = adminCmd("reset-password", os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	b, _ := strconv.ParseBool(os.Getenv(key))
	return b
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return def
}

func openDB(ctx context.Context, dataDir string) (*core.App, func(), error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, nil, err
	}
	conn, err := db.Open(ctx, filepath.Join(dataDir, "blog.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("打开数据库：%w", err)
	}
	return core.New(conn, dataDir, 0), func() { conn.Close() }, nil
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", env("BLOG_ADDR", ":8080"), "监听地址")
	dataDir := fs.String("data", env("BLOG_DATA", "./data"), "数据目录（数据库和附件）")
	maxUpload := fs.Int("max-upload-mb", envInt("BLOG_MAX_UPLOAD_MB", 20), "单个附件大小上限（MB）")
	trustProxy := fs.Bool("trust-proxy", envBool("BLOG_TRUST_PROXY"), "信任反向代理的 X-Forwarded-* 头")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, closeDB, err := openDB(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer closeDB()
	app.MaxUpload = int64(*maxUpload) << 20

	if ok, err := app.AdminExists(ctx); err != nil {
		return err
	} else if !ok {
		slog.Warn("还没有管理员账号，请先运行：blog init-admin -data " + *dataDir)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(app, server.Config{TrustProxy: *trustProxy}, web.Dist()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("服务已启动", "addr", *addr, "data", *dataDir)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("正在关闭…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}

func adminCmd(name string, args []string) error {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	user := fs.String("u", "admin", "用户名")
	dataDir := fs.String("data", env("BLOG_DATA", "./data"), "数据目录")
	_ = fs.Parse(args)

	ctx := context.Background()
	app, closeDB, err := openDB(ctx, *dataDir)
	if err != nil {
		return err
	}
	defer closeDB()

	pw, err := readPassword()
	if err != nil {
		return err
	}
	if name == "init-admin" {
		err = app.CreateAdmin(ctx, *user, pw)
	} else {
		err = app.ResetPassword(ctx, *user, pw)
		if errors.Is(err, core.ErrNotFound) {
			err = fmt.Errorf("用户 %q 不存在", *user)
		}
	}
	if err != nil {
		return err
	}
	fmt.Println("完成。")
	return nil
}

// readPassword reads from BLOG_ADMIN_PASSWORD, the terminal (twice, without
// echo) or a line of stdin, in that order.
func readPassword() (string, error) {
	if pw := os.Getenv("BLOG_ADMIN_PASSWORD"); pw != "" {
		return pw, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("没有读到密码")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Print("密码（至少 8 位）：")
	a, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("再输入一次：")
	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("两次输入的密码不一致")
	}
	return string(a), nil
}
