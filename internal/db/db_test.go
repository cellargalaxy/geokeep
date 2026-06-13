package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"geokeep/internal/db"
	"geokeep/internal/model"

	"gorm.io/gorm"
)

// 验证：1) 文件不存在时自动创建；2) 表自动建立；3) 串行写锁可用。
func TestOpen_AutoCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "geokeep.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("打开失败: %v", err)
	}
	defer d.Close()

	u := &model.User{Email: "a@b.c", PasswordHash: "x", APIKey: "k", Admin: true, Settings: "{}"}
	if err := d.WriteTx(context.Background(), func(tx *gorm.DB) error {
		return tx.Create(u).Error
	}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("自增主键未填充")
	}

	// 唯一约束：重复 email 应失败
	dup := &model.User{Email: "a@b.c", PasswordHash: "y", APIKey: "k2", Settings: "{}"}
	err = d.WriteTx(context.Background(), func(tx *gorm.DB) error { return tx.Create(dup).Error })
	if err == nil {
		t.Fatal("期望 email 唯一约束触发，实际成功")
	}
}

func TestOpen_PointUniqueIndex(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer d.Close()
	u := &model.User{Email: "a@b", PasswordHash: "x", APIKey: "k", Settings: "{}"}
	if err := d.WriteTx(context.Background(), func(tx *gorm.DB) error { return tx.Create(u).Error }); err != nil {
		t.Fatalf("%v", err)
	}
	p := &model.Point{UserID: u.ID, Timestamp: 1, Latitude: 1.1, Longitude: 2.2, Source: "owntracks"}
	if err := d.WriteTx(context.Background(), func(tx *gorm.DB) error { return tx.Create(p).Error }); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	dup := &model.Point{UserID: u.ID, Timestamp: 1, Latitude: 1.1, Longitude: 2.2, Source: "overland"}
	err = d.WriteTx(context.Background(), func(tx *gorm.DB) error { return tx.Create(dup).Error })
	if err == nil {
		t.Fatal("期望 (user_id,lat,lon,ts) 唯一约束触发，实际成功")
	}
}
