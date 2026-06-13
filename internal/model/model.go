// Package model 定义 geokeep 持久化层的 GORM 模型。
// 字段命名与 dawarich db/schema.rb 对齐，便于双向迁移。
// 所有时间统一为 Unix 秒（int64），与 OwnTracks tst 字段一致。
package model

import "time"

// User 单租户场景下通常只有一行；admin=true 的用户拥有所有管理动作权限。
type User struct {
	ID             uint   `gorm:"primaryKey"`
	Email          string `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash   string `gorm:"size:128;not null"`
	APIKey         string `gorm:"size:64;uniqueIndex;not null"` // 上报通道明文 token
	Admin          bool   `gorm:"not null;default:false"`
	Settings       string `gorm:"type:text;default:'{}'"` // JSON 字符串，对齐 dawarich users.settings
	FailedAttempts int    `gorm:"not null;default:0"`
	LockedAt       *int64
	CreatedAt      int64 `gorm:"autoCreateTime"`
	UpdatedAt      int64 `gorm:"autoUpdateTime"`
}

// Device 表示某个用户下的上报设备；OwnTracks 用 tid/topic、Overland 用 device_id 标识。
type Device struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;index:idx_devices_user_name,unique"`
	Name      string `gorm:"size:128;not null;index:idx_devices_user_name,unique"`
	Source    string `gorm:"size:32;not null"` // owntracks / overland / import
	CreatedAt int64  `gorm:"autoCreateTime"`
}

// Point 是一条 GPS 打点记录。
// 唯一约束 (user_id, latitude, longitude, timestamp) 对齐 dawarich 的去重规则。
type Point struct {
	ID               uint    `gorm:"primaryKey"`
	UserID           uint    `gorm:"not null;index:idx_points_user_ts;uniqueIndex:uq_points_user_coord_ts,priority:1"`
	DeviceID         *uint   `gorm:"index:idx_points_user_dev_ts"`
	Timestamp        int64   `gorm:"not null;index:idx_points_user_ts;uniqueIndex:uq_points_user_coord_ts,priority:4"`
	Latitude         float64 `gorm:"not null;uniqueIndex:uq_points_user_coord_ts,priority:2"`
	Longitude        float64 `gorm:"not null;uniqueIndex:uq_points_user_coord_ts,priority:3"`
	Altitude         *float64
	Accuracy         *int
	VerticalAccuracy *int
	Velocity         string // 字符串以对齐 dawarich points.velocity
	Course           *float64
	CourseAccuracy   *float64
	Battery          *int
	BatteryStatus    *int
	Connection       *int
	SSID             string
	BSSID            string
	Trigger          *int
	TrackerID        string `gorm:"size:64;index"`
	Topic            string `gorm:"size:255"`
	InRegions        string `gorm:"type:text"` // JSON 数组
	InRIDs           string `gorm:"type:text"` // JSON 数组
	RawData          []byte `gorm:"type:blob"` // 原始 payload 全保留
	MotionData       []byte `gorm:"type:blob"`
	ImportID         *uint  `gorm:"index"`
	Source           string `gorm:"size:32;not null"` // owntracks / overland / import:<format>
	CreatedAt        int64  `gorm:"autoCreateTime"`
}

// TableName 显式锁定表名，避免 GORM 复数化生成 "points"（其实就是 points，这里只是固化）。
func (Point) TableName() string { return "points" }

// Import 描述一次导入任务的状态机。
type Import struct {
	ID                  uint   `gorm:"primaryKey"`
	UserID              uint   `gorm:"not null;index"`
	Name                string `gorm:"size:255;not null"`
	Source              string `gorm:"size:32;not null"` // dawarich_v1 / dawarich_v2 / owntracks_rec / gpx / geojson
	Status              string `gorm:"size:16;not null"` // pending / running / completed / failed
	FilePath            string `gorm:"size:512;not null"`
	RawPoints           int    `gorm:"not null;default:0"`
	Doubles             int    `gorm:"not null;default:0"`
	Processed           int    `gorm:"not null;default:0"`
	ErrorMessage        string `gorm:"type:text"`
	ProcessingStartedAt *int64
	CreatedAt           int64 `gorm:"autoCreateTime"`
	UpdatedAt           int64 `gorm:"autoUpdateTime"`
}

// Export 描述一次导出任务。
type Export struct {
	ID                  uint   `gorm:"primaryKey"`
	UserID              uint   `gorm:"not null;index"`
	Name                string `gorm:"size:255;not null"`
	FileFormat          string `gorm:"size:32;not null"` // geojson / gpx / owntracks_json / dawarich_v2
	Status              string `gorm:"size:16;not null"` // pending / running / completed / failed / expired
	StartAt             int64
	EndAt               int64
	FilePath            string `gorm:"size:512"`
	FileSize            int64
	ErrorMessage        string `gorm:"type:text"`
	ProcessingStartedAt *int64
	CreatedAt           int64 `gorm:"autoCreateTime"`
	UpdatedAt           int64 `gorm:"autoUpdateTime"`
}

// NowUnix 取当前 Unix 秒；抽出来便于测试 mock。
func NowUnix() int64 { return time.Now().Unix() }
