package ingest

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"geokeep/internal/model"
)

// OverlandBatch 是 Overland HTTP 上报体。
// 来源：https://github.com/aaronpk/Overland-iOS README
type OverlandBatch struct {
	Locations []OverlandFeature `json:"locations"`
	Current   json.RawMessage   `json:"current,omitempty"`
	Trip      json.RawMessage   `json:"trip,omitempty"`
}

// OverlandFeature 是单条 GeoJSON Feature。
type OverlandFeature struct {
	Type       string             `json:"type"`
	Geometry   OverlandGeometry   `json:"geometry"`
	Properties OverlandProperties `json:"properties"`
}

// OverlandGeometry GeoJSON Point。
type OverlandGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"` // [lon, lat]
}

// OverlandProperties Overland 在 Feature.properties 内挂载的字段集合。
type OverlandProperties struct {
	Timestamp          string   `json:"timestamp"` // ISO8601
	Altitude           *float64 `json:"altitude,omitempty"`
	Speed              *float64 `json:"speed,omitempty"` // m/s
	Course             *float64 `json:"course,omitempty"`
	HorizontalAccuracy *int     `json:"horizontal_accuracy,omitempty"`
	VerticalAccuracy   *int     `json:"vertical_accuracy,omitempty"`
	SpeedAccuracy      *float64 `json:"speed_accuracy,omitempty"`
	CourseAccuracy     *float64 `json:"course_accuracy,omitempty"`
	Motion             []string `json:"motion,omitempty"`
	BatteryState       string   `json:"battery_state,omitempty"`
	BatteryLevel       *float64 `json:"battery_level,omitempty"` // 0..1
	Wifi               string   `json:"wifi,omitempty"`
	DeviceID           string   `json:"device_id,omitempty"`
	UniqueID           string   `json:"unique_id,omitempty"`
	Activity           string   `json:"activity,omitempty"`
	Pauses             *bool    `json:"pauses,omitempty"`
	DesiredAccuracy    *int     `json:"desired_accuracy,omitempty"`
	Deferred           *int     `json:"deferred,omitempty"`
	SignificantChange  string   `json:"significant_change,omitempty"`
	LocationsInPayload *int     `json:"locations_in_payload,omitempty"`
}

// MapOverlandBatch 将 Overland batch 拆为多条 Point。
// rawFeatures 与 features 一一对应；保留原始 Feature JSON 到 point.RawData，便于追溯。
func MapOverlandBatch(raw []byte) ([]*model.Point, error) {
	var batch OverlandBatch
	if err := json.Unmarshal(raw, &batch); err != nil {
		return nil, err
	}
	out := make([]*model.Point, 0, len(batch.Locations))
	for _, f := range batch.Locations {
		p, err := mapOverlandFeature(f)
		if err != nil {
			continue // 单条解析失败跳过，不影响整批
		}
		out = append(out, p)
	}
	return out, nil
}

func mapOverlandFeature(f OverlandFeature) (*model.Point, error) {
	if len(f.Geometry.Coordinates) < 2 {
		return nil, errBadCoord
	}
	ts, err := time.Parse(time.RFC3339, f.Properties.Timestamp)
	if err != nil {
		return nil, err
	}
	p := &model.Point{
		Timestamp: ts.Unix(),
		Longitude: f.Geometry.Coordinates[0],
		Latitude:  f.Geometry.Coordinates[1],
		TrackerID: f.Properties.DeviceID,
		SSID:      f.Properties.Wifi,
		Source:    "overland",
	}
	rawFeat, _ := json.Marshal(f)
	p.RawData = rawFeat

	if f.Properties.Altitude != nil {
		v := *f.Properties.Altitude
		p.Altitude = &v
	}
	if f.Properties.HorizontalAccuracy != nil {
		v := *f.Properties.HorizontalAccuracy
		p.Accuracy = &v
	}
	if f.Properties.VerticalAccuracy != nil {
		v := *f.Properties.VerticalAccuracy
		p.VerticalAccuracy = &v
	}
	if f.Properties.Speed != nil {
		// m/s -> km/h 字符串（兼容 dawarich points.velocity 类型 string）
		kmh := int(math.Round(*f.Properties.Speed * 3.6))
		p.Velocity = strconv.Itoa(kmh)
	}
	if f.Properties.Course != nil {
		v := *f.Properties.Course
		p.Course = &v
	}
	if f.Properties.CourseAccuracy != nil {
		v := *f.Properties.CourseAccuracy
		p.CourseAccuracy = &v
	}
	if f.Properties.BatteryLevel != nil {
		v := int(math.Round(*f.Properties.BatteryLevel * 100))
		p.Battery = &v
	}
	if f.Properties.BatteryState != "" {
		v := mapBatteryState(f.Properties.BatteryState)
		p.BatteryStatus = &v
	}
	// motion_data：把 Overland 的运动/活动相关字段聚合
	motion := map[string]any{
		"motion":               f.Properties.Motion,
		"activity":             f.Properties.Activity,
		"pauses":               f.Properties.Pauses,
		"deferred":             f.Properties.Deferred,
		"desired_accuracy":     f.Properties.DesiredAccuracy,
		"significant_change":   f.Properties.SignificantChange,
		"locations_in_payload": f.Properties.LocationsInPayload,
	}
	mb, _ := json.Marshal(motion)
	p.MotionData = mb
	return p, nil
}

// 与 OwnTracks battery_status (bs) 对齐：unknown=0/charging=2/full=3/unplugged=1。
func mapBatteryState(s string) int {
	switch s {
	case "unplugged":
		return 1
	case "charging":
		return 2
	case "full":
		return 3
	}
	return 0
}

var errBadCoord = jsonErr("overland: coordinates 字段缺失")

type jsonErr string

func (e jsonErr) Error() string { return string(e) }
