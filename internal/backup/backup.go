// Package backup 提供基于 SQLite `VACUUM INTO` 的在线热备 + 离线恢复语义。
//
// 热备：触发 `VACUUM INTO <staging>`，SQLite 在内部使用读快照，
// 整个过程不阻塞业务读写；备份完成后流式吐给 HTTP 客户端。
//
// 恢复：仅做离线恢复（CLI 或 web 标记），避免在线热替换的复杂度。
package backup

import (
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
// 强校验：文件存在 + 大小 > 0；不做 PRAGMA integrity_check（GORM 启动期会再校验 schema）。
// 调用此函数前必须确保 db 进程已停止；MVP 用 web 标记文件 + 重启实现「下次启动恢复」。
func Restore(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return errors.New("候选备份文件为空")
	}
	bak := dst + ".bak-" + time.Now().UTC().Format("20060102-150405")
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, bak); err != nil {
			return err
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func randName() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
