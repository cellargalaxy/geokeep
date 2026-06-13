// Package repo 是数据访问层，**所有越权敏感查询统一在此收口**：
// 凡是按 ID 取资源的方法都要求传入 userID 并在 WHERE 子句中下推，
// 防止上层 handler 漏掉 user_id 过滤造成越权读写。
package repo

import (
	"context"
	"errors"
	"strings"

	"geokeep/internal/db"
	"geokeep/internal/model"

	"gorm.io/gorm"
)

// ErrNotFound 资源不存在或越权访问；handler 应统一返回 404，不暴露存在性。
var ErrNotFound = errors.New("资源不存在")

// Repo 包装 *db.DB。所有方法均经 Repo，避免 handler 直接接 GORM。
type Repo struct{ DB *db.DB }

// New 构造 Repo。
func New(d *db.DB) *Repo { return &Repo{DB: d} }

// ===== Users =====

// UserCount 用于「未初始化」检测。
func (r *Repo) UserCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.DB.WithContext(ctx).Model(&model.User{}).Count(&n).Error
	return n, err
}

// CreateUser 创建用户；调用方需先做 email 唯一性与密码强度校验。
func (r *Repo) CreateUser(ctx context.Context, u *model.User) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error { return tx.Create(u).Error })
}

// GetUserByEmail 用于登录路径。
func (r *Repo) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := r.DB.WithContext(ctx).Where("email = ?", strings.ToLower(email)).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

// GetUserByID 仅在 session 中间件内使用；不接受外部 user_id 跨用户查询。
func (r *Repo) GetUserByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	err := r.DB.WithContext(ctx).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

// GetUserByAPIKey 上报通道鉴权使用。
func (r *Repo) GetUserByAPIKey(ctx context.Context, key string) (*model.User, error) {
	var u model.User
	err := r.DB.WithContext(ctx).Where("api_key = ?", key).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &u, err
}

// UpdateAPIKey 轮换 api_key，校验 user_id 防越权。
func (r *Repo) UpdateAPIKey(ctx context.Context, userID uint, newKey string) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&model.User{}).Where("id = ?", userID).Update("api_key", newKey)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// IncrLoginFailure 登录失败计数 +1。
func (r *Repo) IncrLoginFailure(ctx context.Context, userID uint) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		return tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("failed_attempts", gorm.Expr("failed_attempts + 1")).Error
	})
}

// LockUser 在失败次数过多时记录锁定时间。
func (r *Repo) LockUser(ctx context.Context, userID uint, until int64) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		return tx.Model(&model.User{}).Where("id = ?", userID).Update("locked_at", until).Error
	})
}

// ResetLoginFailure 登录成功后清零。
func (r *Repo) ResetLoginFailure(ctx context.Context, userID uint) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		return tx.Model(&model.User{}).Where("id = ?", userID).
			Updates(map[string]any{"failed_attempts": 0, "locked_at": nil}).Error
	})
}

// ===== Devices =====

// UpsertDevice 按 (user_id, name) 唯一约束 upsert。
func (r *Repo) UpsertDevice(ctx context.Context, userID uint, name, source string) (*model.Device, error) {
	if name == "" {
		name = "unknown"
	}
	var d model.Device
	err := r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND name = ?", userID, name).First(&d).Error; err == nil {
			return nil
		}
		d = model.Device{UserID: userID, Name: name, Source: source}
		return tx.Create(&d).Error
	})
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDevices 强制按 user_id 过滤。
func (r *Repo) ListDevices(ctx context.Context, userID uint) ([]model.Device, error) {
	var ds []model.Device
	err := r.DB.WithContext(ctx).Where("user_id = ?", userID).Find(&ds).Error
	return ds, err
}

// ===== Points =====

// InsertPoint 插入单点；触发唯一约束时返回 (false, nil) 表示「dup」。
func (r *Repo) InsertPoint(ctx context.Context, p *model.Point) (inserted bool, err error) {
	err = r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		return tx.Create(p).Error
	})
	if err == nil {
		return true, nil
	}
	if isUniqueErr(err) {
		return false, nil
	}
	return false, err
}

// InsertPointsBatch 批量插入；返回 inserted / dup 计数。
// 用单事务保障「半成功」语义可控（任何非唯一错误立即回滚整批）。
func (r *Repo) InsertPointsBatch(ctx context.Context, ps []*model.Point) (inserted, dup int, err error) {
	err = r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		for _, p := range ps {
			e := tx.Create(p).Error
			if e == nil {
				inserted++
				continue
			}
			if isUniqueErr(e) {
				dup++
				continue
			}
			return e
		}
		return nil
	})
	return
}

// PointQuery 是查询参数；UserID 必填，由 handler 强制注入。
type PointQuery struct {
	UserID    uint
	From      int64
	To        int64
	DeviceIDs []uint
	Limit     int
	Sample    int // 等步长抽样：每 N 个返回 1 个，<=1 表示不抽样
}

// QueryPoints 按时间窗 + 设备过滤；user_id 强制带入 WHERE。
func (r *Repo) QueryPoints(ctx context.Context, q PointQuery) ([]model.Point, error) {
	var raw []model.Point
	err := r.QueryPointsStream(ctx, q, func(p model.Point) error {
		raw = append(raw, p)
		return nil
	})
	return raw, err
}

// QueryPointsStream 流式查询；每读到一条 Point 记录即调用 fn。
// 适用于大批量数据导出或聚合场景。
func (r *Repo) QueryPointsStream(ctx context.Context, q PointQuery, fn func(model.Point) error) error {
	if q.UserID == 0 {
		return errors.New("QueryPointsStream: user_id 必填，防止越权")
	}
	tx := r.DB.WithContext(ctx).Model(&model.Point{}).
		Where("user_id = ?", q.UserID)
	if q.From > 0 {
		tx = tx.Where("timestamp >= ?", q.From)
	}
	if q.To > 0 {
		tx = tx.Where("timestamp <= ?", q.To)
	}
	if len(q.DeviceIDs) > 0 {
		tx = tx.Where("device_id IN ?", q.DeviceIDs)
	}
	tx = tx.Order("timestamp ASC")

	rows, err := tx.Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var p model.Point
		if err := r.DB.ScanRows(rows, &p); err != nil {
			return err
		}
		count++
		if q.Sample > 1 && (count-1)%q.Sample != 0 {
			continue
		}
		if err := fn(p); err != nil {
			return err
		}
		if q.Limit > 0 && count >= q.Limit {
			break
		}
	}
	return nil
}

// CountPoints 仅统计用户自己的点。
func (r *Repo) CountPoints(ctx context.Context, userID uint) (int64, error) {
	var n int64
	err := r.DB.WithContext(ctx).Model(&model.Point{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

// ===== Imports / Exports =====

// CreateImport 落 Import 任务。
func (r *Repo) CreateImport(ctx context.Context, im *model.Import) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error { return tx.Create(im).Error })
}

// GetImport 越权防护：必须按 user_id 过滤。
func (r *Repo) GetImport(ctx context.Context, userID, id uint) (*model.Import, error) {
	var im model.Import
	err := r.DB.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&im).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &im, err
}

// UpdateImport 更新可变字段，写时强校验 user_id。
func (r *Repo) UpdateImport(ctx context.Context, userID, id uint, fields map[string]any) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&model.Import{}).Where("user_id = ? AND id = ?", userID, id).Updates(fields)
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return res.Error
	})
}

// CreateExport 落 Export 任务。
func (r *Repo) CreateExport(ctx context.Context, ex *model.Export) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error { return tx.Create(ex).Error })
}

// GetExport 越权防护：必须按 user_id 过滤。
func (r *Repo) GetExport(ctx context.Context, userID, id uint) (*model.Export, error) {
	var ex model.Export
	err := r.DB.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&ex).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &ex, err
}

// UpdateExport 更新可变字段，写时强校验 user_id。
func (r *Repo) UpdateExport(ctx context.Context, userID, id uint, fields map[string]any) error {
	return r.DB.WriteTx(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&model.Export{}).Where("user_id = ? AND id = ?", userID, id).Updates(fields)
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return res.Error
	})
}

// ListImports 列出用户自己的导入任务。
func (r *Repo) ListImports(ctx context.Context, userID uint) ([]model.Import, error) {
	var xs []model.Import
	err := r.DB.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Find(&xs).Error
	return xs, err
}

// ListExports 列出用户自己的导出任务。
func (r *Repo) ListExports(ctx context.Context, userID uint) ([]model.Export, error) {
	var xs []model.Export
	err := r.DB.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Find(&xs).Error
	return xs, err
}

// isUniqueErr 适配 glebarez/sqlite（modernc.org/sqlite）的唯一约束错误文本。
// 错误信息通常为 "constraint failed: UNIQUE constraint failed: ..."。
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed: UNIQUE")
}
