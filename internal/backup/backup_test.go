package backup_test

import (
	"bytes"
	"context"
	"os"
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
	makeSQLiteDB(t, src, "src@x", "ksrc")
	makeSQLiteDB(t, dst, "old@x", "kold")

	if err := backup.Restore(src, dst); err != nil {
		t.Fatal(err)
	}
	content, _ := read(dst)
	if !bytes.HasPrefix(content, []byte("SQLite format 3")) {
		t.Fatalf("dst 未被覆盖为 SQLite: %q", content[:16])
	}
	restored, err := db.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var n int64
	if err := restored.Model(&model.User{}).Where("email = ?", "src@x").Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("恢复后应包含源库用户: n=%d err=%v", n, err)
	}
}

func TestRestore_InvalidSQLiteDoesNotReplaceExistingDB(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.db")
	dst := filepath.Join(dir, "dst.db")
	if err := write(src, []byte("SQLite format 3\x00fake")); err != nil {
		t.Fatal(err)
	}
	makeSQLiteDB(t, dst, "old@x", "kold")
	before, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := backup.Restore(src, dst); err == nil {
		t.Fatal("损坏 SQLite 文件应被拒绝")
	}
	after, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("恢复失败时不应替换现有数据库")
	}
}

func makeSQLiteDB(t *testing.T, path, email, key string) {
	t.Helper()
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u := &model.User{Email: email, PasswordHash: "x", APIKey: key, Settings: "{}"}
	if err := d.WriteTx(context.Background(), func(tx *gorm.DB) error { return tx.Create(u).Error }); err != nil {
		t.Fatal(err)
	}
}

func write(p string, b []byte) error {
	return writeFile(p, b)
}
