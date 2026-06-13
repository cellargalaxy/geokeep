package importer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"geokeep/internal/model"
)

// geoJSONParser 支持 FeatureCollection<Point> 与 Feature<LineString>。
// LineString 必须提供 properties.coordTimes，时间戳数量必须与坐标数量一致，不做推断。
type geoJSONParser struct{}

type gjTop struct {
	Type string `json:"type"`
}

type gjFeatureCollection struct {
	Type     string            `json:"type"`
	Features []json.RawMessage `json:"features"`
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
	var top gjTop
	if err := json.Unmarshal(raw, &top); err != nil {
		return err
	}
	switch top.Type {
	case "FeatureCollection":
		var fc gjFeatureCollection
		if err := json.Unmarshal(raw, &fc); err != nil {
			return err
		}
		for _, fraw := range fc.Features {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := parseGeoJSONFeature(ctx, fraw, emit); err != nil {
				return err
			}
		}
		return nil
	case "Feature":
		return parseGeoJSONFeature(ctx, raw, emit)
	default:
		return errors.New("geojson: 仅支持 FeatureCollection 或 Feature 顶层类型")
	}
}

func parseGeoJSONFeature(ctx context.Context, raw json.RawMessage, emit func(*model.Point) error) error {
	var f gjFeature
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	if f.Type != "Feature" {
		return errors.New("geojson: features 内仅支持 Feature")
	}
	var props map[string]json.RawMessage
	if len(f.Properties) > 0 {
		if err := json.Unmarshal(f.Properties, &props); err != nil {
			return err
		}
	} else {
		props = map[string]json.RawMessage{}
	}
	switch f.Geometry.Type {
	case "Point":
		var coord []float64
		if err := json.Unmarshal(f.Geometry.Coordinates, &coord); err != nil {
			return err
		}
		if len(coord) < 2 {
			return nil
		}
		ts, err := extractTimestamp(props)
		if err != nil {
			return err
		}
		pt := pointFromCoord(coord, ts, raw)
		return emit(pt)
	case "LineString":
		var coords [][]float64
		if err := json.Unmarshal(f.Geometry.Coordinates, &coords); err != nil {
			return err
		}
		times, err := extractCoordTimes(props)
		if err != nil {
			return err
		}
		if len(coords) != len(times) {
			return errors.New("geojson: LineString coordTimes 数量必须与 coordinates 一致")
		}
		for i, coord := range coords {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if len(coord) < 2 {
				continue
			}
			if err := emit(pointFromCoord(coord, times[i], raw)); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("geojson: 仅支持 Point 或 LineString")
	}
}

func pointFromCoord(coord []float64, ts int64, raw json.RawMessage) *model.Point {
	pt := &model.Point{Timestamp: ts, Longitude: coord[0], Latitude: coord[1], RawData: append([]byte(nil), raw...)}
	if len(coord) >= 3 {
		alt := coord[2]
		pt.Altitude = &alt
	}
	return pt
}

// extractTimestamp 优先识别 properties.timestamp / properties.ts / properties.time。
// 字符串走 RFC3339；整数/浮点数当 epoch 秒。无法解析直接报错（不脑补）。
func extractTimestamp(props map[string]json.RawMessage) (int64, error) {
	for _, key := range []string{"timestamp", "ts", "time"} {
		v, ok := props[key]
		if !ok {
			continue
		}
		if ts, err := parseTimeValue(v); err == nil {
			return ts, nil
		}
	}
	return 0, errors.New("geojson: feature.properties 缺失可识别的时间戳字段 (timestamp/ts/time)")
}

func extractCoordTimes(props map[string]json.RawMessage) ([]int64, error) {
	raw, ok := props["coordTimes"]
	if !ok {
		return nil, errors.New("geojson: LineString 必须提供 properties.coordTimes")
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(values))
	for _, v := range values {
		ts, err := parseTimeValue(v)
		if err != nil {
			return nil, errors.New("geojson: coordTimes 含无法解析的时间戳")
		}
		out = append(out, ts)
	}
	return out, nil
}

func parseTimeValue(v json.RawMessage) (int64, error) {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		t, err := time.Parse(time.RFC3339, s)
		if err == nil {
			return t.Unix(), nil
		}
		return 0, err
	}
	var n int64
	if err := json.Unmarshal(v, &n); err == nil {
		return n, nil
	}
	var f float64
	if err := json.Unmarshal(v, &f); err == nil {
		return int64(f), nil
	}
	return 0, errors.New("bad time")
}
