package importer

import (
	"context"
	"io"

	"geokeep/internal/model"

	"github.com/tkrajina/gpxgo/gpx"
)

// gpxParser 解析 GPX 1.0/1.1。
// 不支持流式（gpxgo 不暴露 Reader 接口），但 MVP 个人 2-3 设备场景文件不会过大；
// 如未来需要超大 GPX，可切换 SAX-based 解析（XML decoder）。
type gpxParser struct{}

func (p *gpxParser) Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	g, err := gpx.ParseBytes(raw)
	if err != nil {
		return err
	}
	for _, trk := range g.Tracks {
		for _, seg := range trk.Segments {
			for _, pt := range seg.Points {
				if ctx.Err() != nil {
					return ctx.Err()
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
				if err := emit(p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
