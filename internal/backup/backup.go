// Package backup 提供基于 SQLite `VACUUM INTO` 的在线热备 + 离线恢复语义。
//
// 热备：触发 `VACUUM INTO <staging>`，SQLite 在内部使用读快照，
// 整个过程不阻塞业务读写；备份完成后流式吐给 HTTP 客户端。
//
// 恢复：仅做离线恢复（CLI 或 web 标记），避免在线热替换的复杂度。
package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"geokeep/internal/db"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Service 持有 db 与备份目录。
type Service struct {
	db  *db.DB
	dir string
}

// New 构造 Service。
func New(d *db.DB, dir string) *Service { return &Service{db: d, dir: dir} }

// VacuumInto 触发一次 `VACUUM INTO`，返回备份文件路径。
// 调用方应在使用完文件后 os.Remove，或交给保留策略。
func (s *Service) VacuumInto(ctx context.Context) (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", err
	}
	nonce, err := randName()
	if err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102-150405")
	path := filepath.Join(s.dir, fmt.Sprintf("geokeep-%s-%s.db", ts, nonce))
	// SQLite VACUUM INTO 要求绑定字面量字符串
	if err := s.db.WithContext(ctx).Exec("VACUUM INTO ?", path).Error; err != nil {
		return "", err
	}
	return path, nil
}

// StreamBackup 一次性走 VACUUM INTO + 流式拷贝到 w，结束后删除临时文件。
func (s *Service) StreamBackup(ctx context.Context, w io.Writer) (int64, error) {
	path, err := s.VacuumInto(ctx)
	if err != nil {
		return 0, err
	}
	defer os.Remove(path)
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return io.Copy(w, f)
}

// Restore 把候选 .db 文件覆盖到 dst。
// 强校验：SQLite 文件头 + PRAGMA integrity_check；失败时不会移动现有 dst。
// 调用此函数前必须确保 db 进程已停止；MVP 用 web 标记文件 + 重启实现「下次启动恢复」。
func Restore(src, dst string) error {
	if samePath(src, dst) {
		return errors.New("源文件与目标数据库相同")
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return errors.New("候选备份文件为空")
	}
	if err := validateSQLite(src); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}

	bak := dst + ".bak-" + time.Now().UTC().Format("20060102-150405")
	renamed := false
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, bak); err != nil {
			return err
		}
		renamed = true
	}
	success := false
	defer func() {
		if success {
			return
		}
		_ = os.Remove(dst)
		if renamed {
			_ = os.Rename(bak, dst)
		}
	}()

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	success = true
	return nil
}

func validateSQLite(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	magic := make([]byte, 16)
	_, err = io.ReadFull(f, magic)
	_ = f.Close()
	if err != nil {
		return err
	}
	if !bytes.Equal(magic, []byte("SQLite format 3\x00")) {
		return errors.New("候选备份不是 SQLite 数据库文件")
	}
	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("打开候选备份失败: %w", err)
	}
	sqlDB, dbErr := gdb.DB()
	if dbErr == nil {
		defer sqlDB.Close()
	}
	var result string
	if err := gdb.Raw("PRAGMA integrity_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("integrity_check 失败: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check 非 ok: %s", result)
	}
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}

func randName() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
