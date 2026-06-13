package importer

import (
	"context"
	"encoding/json"
	"io"

	"geokeep/internal/model"

	"github.com/tkrajina/gpxgo/gpx"
)

// gpxParser 解析 GPX 1.0/1.1。
// 不支持流式（gpxgo 不暴露 Reader 接口），但 MVP 个人 2-3 设备场景文件不会过大；
// 如未来需要超大 GPX，可切换 SAX-based 解析（XML decoder）。
type gpxParser struct{}

func (p *gpxParser) Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	g, err := gpx.Parse(r)
	if err != nil {
		return err
	}
	for _, trk := range g.Tracks {
		for _, seg := range trk.Segments {
			for _, pt := range seg.Points {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// 「不脑补时间戳」原则：缺失 <time> 的 trkpt 跳过，避免 Unix(0001-01-01) 这种伪值落库。
				if pt.Timestamp.IsZero() {
					continue
				}
				p := &model.Point{
					Timestamp: pt.Timestamp.Unix(),
					Latitude:  pt.Latitude,
					Longitude: pt.Longitude,
				}
				if pt.Elevation.NotNull() {
					v := pt.Elevation.Value()
					p.Altitude = &v
				}
				// raw_data 保留：与其它 importer 一致，序列化 trkpt 关键字段。
				raw := map[string]any{
					"lat":  pt.Latitude,
					"lon":  pt.Longitude,
					"time": pt.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
				}
				if pt.Elevation.NotNull() {
					raw["ele"] = pt.Elevation.Value()
				}
				if b, err := json.Marshal(raw); err == nil {
					p.RawData = b
				}
				if err := emit(p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
