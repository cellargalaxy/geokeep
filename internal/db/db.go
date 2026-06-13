// Package db 负责 SQLite 打开、PRAGMA 应用、GORM AutoMigrate 与写锁。
//
// 写并发模型：SQLite WAL 允许多读单写；本包通过 sync.Mutex 把所有写事务
// 串行化到 db.WriteTx 入口，避免 database/sql 触发 SQLITE_BUSY 重试风暴。
package db

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"geokeep/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 是包装后的 GORM DB，对外提供 WriteTx 串行写事务入口。
type DB struct {
	*gorm.DB
	writeMu sync.Mutex
}

// Open 打开 SQLite 文件并应用 PRAGMA + AutoMigrate。
// path 为 SQLite 文件路径，不存在则自动创建。
func Open(path string) (*DB, error) {
	gdb, err := gorm.Open(sqlite.Open(path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	if err := applyPragmas(gdb); err != nil {
		return nil, err
	}
	if err := AutoMigrate(gdb); err != nil {
		return nil, fmt.Errorf("AutoMigrate 失败: %w", err)
	}
	return &DB{DB: gdb}, nil
}

// AutoMigrate 创建所有 geokeep 业务表。
// 实现「自动建表」需求：表不存在则建，表存在则按 GORM 兼容规则增列。
func AutoMigrate(gdb *gorm.DB) error {
	return gdb.AutoMigrate(
		&model.User{},
		&model.Device{},
		&model.Point{},
		&model.Import{},
		&model.Export{},
	)
}

func applyPragmas(gdb *gorm.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -65536",
	}
	for _, p := range pragmas {
		if err := gdb.Exec(p).Error; err != nil {
			return fmt.Errorf("应用 PRAGMA 失败 %q: %w", p, err)
		}
	}
	return nil
}

// WriteTx 在串行写锁内执行 fn；fn 内的所有写操作必须使用传入的 tx。
// fn 返回 error 时事务回滚；context 取消时立即返回，不等待 fn 结束。
func (d *DB) WriteTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.WithContext(ctx).Transaction(fn)
}

// Close 关闭底层 sql.DB。
func (d *DB) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ErrConflict 用于上层判定唯一约束冲突。
var ErrConflict = errors.New("unique conflict")
