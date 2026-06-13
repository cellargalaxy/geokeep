// geokeep 服务入口。
// 子命令：
//
//	serve              （默认）启动 HTTP 服务
//	rotate-key --email 离线重置某账号的 api_key
//	restore --from     离线恢复 SQLite 备份文件
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"geokeep/internal/auth"
	"geokeep/internal/backup"
	"geokeep/internal/config"
	"geokeep/internal/db"
	"geokeep/internal/repo"
	"geokeep/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !startsWithDash(args[0]) {
		cmd = args[0]
		args = args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "rotate-key":
		err = runRotateKey(args)
	case "restore":
		err = runRestore(args)
	default:
		err = fmt.Errorf("未知子命令: %s", cmd)
	}
	if err != nil {
		slog.Error("命令失败", "cmd", cmd, "err", err)
		os.Exit(1)
	}
}

func startsWithDash(s string) bool { return len(s) > 0 && s[0] == '-' }

func runServe(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	if err := consumePendingRestore(cfg); err != nil {
		return err
	}
	d, err := db.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()
	r := repo.New(d)
	srv := server.New(cfg, d, r)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.Manager().Start(ctx)
	srv.ExportManager().Start(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("geokeep listening", "addr", cfg.Listen, "base_path", cfg.BasePath, "data_dir", cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http serve", "err", err)
			cancel()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		slog.Info("收到停机信号")
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	srv.Manager().Stop()
	srv.ExportManager().Stop()
	return nil
}

func runRotateKey(args []string) error {
	fs := flag.NewFlagSet("rotate-key", flag.ContinueOnError)
	email := fs.String("email", "", "要轮换 API Key 的账号邮箱")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email 必填")
	}
	cfg, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	d, err := db.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()
	r := repo.New(d)
	u, err := r.GetUserByEmail(context.Background(), *email)
	if err != nil {
		return err
	}
	newKey, err := auth.GenerateAPIKey()
	if err != nil {
		return err
	}
	if err := r.UpdateAPIKey(context.Background(), u.ID, newKey); err != nil {
		return err
	}
	fmt.Printf("新 API Key（请妥善保存，仅显示一次）:\n%s\n", newKey)
	return nil
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	from := fs.String("from", "", "候选 .db 备份文件路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" {
		return errors.New("--from 必填")
	}
	cfg, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	if err := backup.Restore(*from, cfg.DBPath()); err != nil {
		return err
	}
	fmt.Println("恢复完成。下次启动 serve 时将使用新数据库。")
	return nil
}

// loadConfigForCLI 取最小集合：仅 DataDir / Secret 必须；其它默认即可。
func loadConfigForCLI() (*config.Config, error) {
	// 复用 config.Load，但只关心 data-dir / secret
	cfg, err := config.Load([]string{})
	return cfg, err
}

// consumePendingRestore 启动期处理 web restore 上传的标记文件。
func consumePendingRestore(cfg *config.Config) error {
	flagPath := filepath.Join(cfg.DataDir, ".pending_restore")
	data, err := os.ReadFile(flagPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	src := string(data)
	if src == "" {
		_ = os.Remove(flagPath)
		return nil
	}
	slog.Info("执行待恢复", "src", src, "dst", cfg.DBPath())
	if err := backup.Restore(src, cfg.DBPath()); err != nil {
		slog.Error("恢复失败", "err", err)
		return err
	}
	_ = os.Remove(flagPath)
	_ = os.Remove(src)
	return nil
}
