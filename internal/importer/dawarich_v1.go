package importer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"geokeep/internal/model"
)

// dawarichV1Parser 解析 dawarich v1 archive 内的 data.json（流式 token 模式，避免 OOM）。
// 顶层期望对象内含 "points": [ ... ]；其它键忽略。
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
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		var row dawarichPoint
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		p := row.toModel()
		if p == nil {
			continue
		}
		p.RawData = append([]byte(nil), raw...)
		if err := emit(p); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil { // 读 ']'
		return err
	}
	return nil
}

// dawarichPoint 对齐 dawarich points 表常用列；未知字段由调用方把整行 JSON 放进 raw_data 保留。
type dawarichPoint struct {
	Latitude         float64         `json:"latitude"`
	Longitude        float64         `json:"longitude"`
	Timestamp        int64           `json:"timestamp"`
	Altitude         *float64        `json:"altitude"`
	AltitudeDecimal  *float64        `json:"altitude_decimal"`
	Accuracy         *int            `json:"accuracy"`
	VerticalAccuracy *int            `json:"vertical_accuracy"`
	Velocity         json.RawMessage `json:"velocity"`
	Course           *float64        `json:"course"`
	CourseAccuracy   *float64        `json:"course_accuracy"`
	Battery          *int            `json:"battery"`
	BatteryStatus    *int            `json:"battery_status"`
	Connection       *int            `json:"connection"`
	SSID             string          `json:"ssid"`
	BSSID            string          `json:"bssid"`
	Trigger          *int            `json:"trigger"`
	TrackerID        string          `json:"tracker_id"`
	Topic            string          `json:"topic"`
	InRegions        json.RawMessage `json:"in_regions"`
	InRIDs           json.RawMessage `json:"inrids"`
	MotionData       json.RawMessage `json:"motion_data"`
}

func (d dawarichPoint) toModel() *model.Point {
	if d.Timestamp == 0 {
		return nil
	}
	p := &model.Point{
		Timestamp: d.Timestamp,
		Latitude:  d.Latitude,
		Longitude: d.Longitude,
		Velocity:  parseVelocity(d.Velocity),
		TrackerID: d.TrackerID,
		Topic:     d.Topic,
		SSID:      d.SSID,
		BSSID:     d.BSSID,
	}
	if d.Altitude != nil {
		v := *d.Altitude
		p.Altitude = &v
	} else if d.AltitudeDecimal != nil {
		v := *d.AltitudeDecimal
		p.Altitude = &v
	}
	if d.Accuracy != nil {
		v := *d.Accuracy
		p.Accuracy = &v
	}
	if d.VerticalAccuracy != nil {
		v := *d.VerticalAccuracy
		p.VerticalAccuracy = &v
	}
	if d.Course != nil {
		v := *d.Course
		p.Course = &v
	}
	if d.CourseAccuracy != nil {
		v := *d.CourseAccuracy
		p.CourseAccuracy = &v
	}
	if d.Battery != nil {
		v := *d.Battery
		p.Battery = &v
	}
	if d.BatteryStatus != nil {
		v := *d.BatteryStatus
		p.BatteryStatus = &v
	}
	if d.Connection != nil {
		v := *d.Connection
		p.Connection = &v
	}
	if d.Trigger != nil {
		v := *d.Trigger
		p.Trigger = &v
	}
	if rawJSONPresent(d.InRegions) {
		p.InRegions = string(d.InRegions)
	}
	if rawJSONPresent(d.InRIDs) {
		p.InRIDs = string(d.InRIDs)
	}
	if rawJSONPresent(d.MotionData) {
		p.MotionData = append([]byte(nil), d.MotionData...)
	}
	return p
}

func parseVelocity(raw json.RawMessage) string {
	if !rawJSONPresent(raw) {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return n.String()
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return ""
}

func rawJSONPresent(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

// skipValue 让 decoder 消费一个完整 JSON 值（对象/数组/标量），跳过未知字段。
func skipValue(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if _, isDelim := t.(json.Delim); !isDelim {
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
	return nil
}
