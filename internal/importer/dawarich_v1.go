package importer

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"geokeep/internal/model"
)

// dawarichV1Parser 解析 dawarich v1 archive 内的 data.json（流式 token 模式，避免 OOM）。
// 顶层期望对象内含 "points": [ ... ]；其它键忽略（保 raw_data 在 v2 路径）。
type dawarichV1Parser struct{}

func (p *dawarichV1Parser) Parse(ctx context.Context, r io.Reader, emit func(*model.Point) error) error {
	dec := json.NewDecoder(r)
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return errors.New("dawarich_v1: 顶层必须是对象")
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		if k, ok := key.(string); ok && k == "points" {
			if err := readPointsArray(ctx, dec, emit); err != nil {
				return err
			}
			continue
		}
		// 跳过非 points 节点的整体值
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	return nil
}

func readPointsArray(ctx context.Context, dec *json.Decoder, emit func(*model.Point) error) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '[' {
		return errors.New("dawarich_v1: points 字段必须是数组")
	}
	for dec.More() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var row dawarichPoint
		if err := dec.Decode(&row); err != nil {
			return err
		}
		p := row.toModel()
		if p == nil {
			continue
		}
		if err := emit(p); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil { // 读 ']'
		return err
	}
	return nil
}

// dawarichPoint 对齐 dawarich points 表常用列；未知字段忽略，但整体 JSON 保留到 raw_data。
type dawarichPoint struct {
	Latitude  float64  `json:"latitude"`
	Longitude float64  `json:"longitude"`
	Timestamp int64    `json:"timestamp"`
	Altitude  *float64 `json:"altitude"`
	Accuracy  *int     `json:"accuracy"`
	Velocity  string   `json:"velocity"`
	Battery   *int     `json:"battery"`
	TrackerID string   `json:"tracker_id"`
	Topic     string   `json:"topic"`
}

func (d dawarichPoint) toModel() *model.Point {
	if d.Timestamp == 0 {
		return nil
	}
	p := &model.Point{
		Timestamp: d.Timestamp,
		Latitude:  d.Latitude,
		Longitude: d.Longitude,
		Velocity:  d.Velocity,
		TrackerID: d.TrackerID,
		Topic:     d.Topic,
	}
	if d.Altitude != nil {
		v := *d.Altitude
		p.Altitude = &v
	}
	if d.Accuracy != nil {
		v := *d.Accuracy
		p.Accuracy = &v
	}
	if d.Battery != nil {
		v := *d.Battery
		p.Battery = &v
	}
	return p
}

// skipValue 让 decoder 消费一个完整 JSON 值（对象/数组/标量），跳过未知字段。
func skipValue(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	d, isDelim := t.(json.Delim)
	if !isDelim {
		return nil
	}
	depth := 1
	for depth > 0 {
		tk, err := dec.Token()
		if err != nil {
			return err
		}
		if dl, ok := tk.(json.Delim); ok {
			switch dl {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	_ = d
	return nil
}
