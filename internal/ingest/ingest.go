package ingest

import (
	"context"

	"geokeep/internal/model"
	"geokeep/internal/repo"
)

// Sink 是 ingest 写入端抽象，便于测试 mock。
type Sink interface {
	UpsertDevice(ctx context.Context, userID uint, name, source string) (*model.Device, error)
	InsertPointsBatch(ctx context.Context, ps []*model.Point) (inserted, dup int, err error)
}

// Ingestor 串联「mapper -> 设备 upsert -> 批量写库」。
type Ingestor struct{ Repo *repo.Repo }

// New 构造 Ingestor。
func New(r *repo.Repo) *Ingestor { return &Ingestor{Repo: r} }

// SaveOwnTracks 写一条 OwnTracks location。
// 返回 (inserted, dup, err)：inserted/dup 为 0/1。
func (i *Ingestor) SaveOwnTracks(ctx context.Context, userID uint, raw []byte) (int, int, error) {
	p, err := MapOwnTracksLocation(raw)
	if err != nil {
		return 0, 0, err
	}
	return i.saveWithDevice(ctx, userID, p, deviceName(p), "owntracks")
}

// SaveOverland 写一批 Overland Feature。
func (i *Ingestor) SaveOverland(ctx context.Context, userID uint, raw []byte) (int, int, error) {
	ps, err := MapOverlandBatch(raw)
	if err != nil {
		return 0, 0, err
	}
	insTotal, dupTotal := 0, 0
	for _, p := range ps {
		ins, dup, err := i.saveWithDevice(ctx, userID, p, deviceName(p), "overland")
		if err != nil {
			return insTotal, dupTotal, err
		}
		insTotal += ins
		dupTotal += dup
	}
	return insTotal, dupTotal, nil
}

func (i *Ingestor) saveWithDevice(ctx context.Context, userID uint, p *model.Point, name, source string) (int, int, error) {
	p.UserID = userID
	if name != "" {
		dev, err := i.Repo.UpsertDevice(ctx, userID, name, source)
		if err != nil {
			return 0, 0, err
		}
		p.DeviceID = &dev.ID
	}
	ins, dup, err := i.Repo.InsertPointsBatch(ctx, []*model.Point{p})
	return ins, dup, err
}

// deviceName 优先 TrackerID（OwnTracks tid），其次 Topic 末段。
func deviceName(p *model.Point) string {
	if p.TrackerID != "" {
		return p.TrackerID
	}
	if p.Topic != "" {
		return p.Topic
	}
	return ""
}
