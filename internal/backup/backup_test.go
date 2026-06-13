package backup_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"geokeep/internal/backup"
	"geokeep/internal/db"
	"geokeep/internal/model"

	"gorm.io/gorm"
)

func TestVacuumInto_FilePresent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "g.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// 写入一条记录使数据库非空
	u := &model.User{Email: "a@b", PasswordHash: "x", APIKey: "k", Settings: "{}"}
	if err := d.WriteTx(context.Background(), func(tx *gorm.DB) error { return tx.Create(u).Error }); err != nil {
		t.Fatal(err)
	}
	s := backup.New(d, filepath.Join(dir, "backups"))
	var buf bytes.Buffer
	n, err := s.StreamBackup(context.Background(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 || buf.Len() == 0 {
		t.Fatal("备份字节数为 0")
	}
	// SQLite 文件起始 magic: "SQLite format 3\x00"
	if !bytes.HasPrefix(buf.Bytes(), []byte("SQLite format 3")) {
		t.Fatalf("备份内容不是 SQLite 文件: %q", buf.Bytes()[:16])
	}
}

func TestRestore_BackupAndCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	dst := filepath.Join(dir, "dst.db")
	// 准备 src
	if err := write(src, []byte("SQLite format 3\x00fake")); err != nil {
		t.Fatal(err)
	}
	if err := write(dst, []byte("oldcontent")); err != nil {
		t.Fatal(err)
	}
	if err := backup.Restore(src, dst); err != nil {
		t.Fatal(err)
	}
	content, _ := read(dst)
	if !bytes.HasPrefix(content, []byte("SQLite format 3")) {
		t.Fatalf("dst 未被覆盖: %q", content[:16])
	}
}

func write(p string, b []byte) error {
	return writeFile(p, b)
}
