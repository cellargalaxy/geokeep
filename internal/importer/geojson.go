package importer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"geokeep/internal/model"
)

// geoJSONParser 仅接受 FeatureCollection<Point>，properties 含 timestamp（ISO8601 或 epoch 秒）。
// LineString 等其它几何类型不脑补时间戳，整体拒绝。
type geoJSONParser struct{}

type gjFeatureCollection struct {
	Type     string      `json:"type"`
	Features []gjFeature `json:"features"`
}

type gjFeature struct {
	Type       string          `json:"type"`
	Geometry   gjGeometry      `json:"geometry"`
	Properties json.RawMessage `json:"properties"`
}

type gjGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

func (p *geoJSONParser) Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	var fc gjFeatureCollection
	if err := json.Unmarshal(raw, &fc); err != nil {
		return err
	}
	if fc.Type != "FeatureCollection" {
		return errors.New("geojson: 仅支持 FeatureCollection 顶层类型")
	}
	for _, f := range fc.Features {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if f.Geometry.Type != "Point" {
			return errors.New("geojson: 仅支持 Feature.geometry=Point")
		}
		var coord []float64
		if err := json.Unmarshal(f.Geometry.Coordinates, &coord); err != nil {
			return err
		}
		if len(coord) < 2 {
			continue
		}
		var props map[string]json.RawMessage
		if err := json.Unmarshal(f.Properties, &props); err != nil {
			return err
		}
		ts, err := extractTimestamp(props)
		if err != nil {
			return err
		}
		pt := &model.Point{
			Timestamp: ts,
			Longitude: coord[0],
			Latitude:  coord[1],
		}
		if err := emit(pt); err != nil {
			return err
		}
	}
	return nil
}

// extractTimestamp 优先识别 properties.timestamp / properties.ts / properties.time。
// 字符串走 RFC3339；整数当 epoch 秒。无法解析直接报错（不脑补）。
func extractTimestamp(props map[string]json.RawMessage) (int64, error) {
	for _, key := range []string{"timestamp", "ts", "time"} {
		v, ok := props[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			t, err := time.Parse(time.RFC3339, s)
			if err == nil {
				return t.Unix(), nil
			}
		}
		var n int64
		if err := json.Unmarshal(v, &n); err == nil {
			return n, nil
		}
	}
	return 0, errors.New("geojson: feature.properties 缺失可识别的时间戳字段 (timestamp/ts/time)")
}
