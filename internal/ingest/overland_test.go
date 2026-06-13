package ingest_test

import (
	"encoding/json"
	"testing"

	"geokeep/internal/ingest"
)

const sampleOverland = `{
  "locations": [
    {
      "type": "Feature",
      "geometry": {"type":"Point","coordinates":[2.3522, 48.8566]},
      "properties": {
        "timestamp": "2024-01-02T03:04:05Z",
        "altitude": 35.5,
        "speed": 10.0,
        "course": 180.0,
        "horizontal_accuracy": 5,
        "vertical_accuracy": 3,
        "battery_level": 0.42,
        "battery_state": "charging",
        "wifi": "Home",
        "device_id": "iPhone-Alice",
        "motion": ["walking"],
        "activity": "fitness"
      }
    },
    {
      "type": "Feature",
      "geometry": {"type":"Point","coordinates":[2.3, 48.8]},
      "properties": {
        "timestamp": "2024-01-02T03:04:10Z"
      }
    }
  ],
  "current": {"x":1},
  "trip": {"y":2}
}`

func TestMapOverlandBatch(t *testing.T) {
	ps, err := ingest.MapOverlandBatch([]byte(sampleOverland))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(ps) != 2 {
		t.Fatalf("应有 2 条: %d", len(ps))
	}
	p := ps[0]
	if p.Timestamp != 1704164645 { // 2024-01-02T03:04:05Z
		t.Errorf("timestamp 错: %d", p.Timestamp)
	}
	if p.Latitude != 48.8566 || p.Longitude != 2.3522 {
		t.Errorf("coord 顺序错 (lon,lat): %v %v", p.Latitude, p.Longitude)
	}
	if p.Velocity != "36" { // 10 m/s -> 36 km/h
		t.Errorf("速度 m/s -> km/h 转换错: %q", p.Velocity)
	}
	if p.Battery == nil || *p.Battery != 42 { // 0.42 -> 42
		t.Errorf("battery 0.42 应转 42: %v", p.Battery)
	}
	if p.BatteryStatus == nil || *p.BatteryStatus != 2 { // charging=2
		t.Errorf("battery_state charging 应映射 2: %v", p.BatteryStatus)
	}
	if p.TrackerID != "iPhone-Alice" {
		t.Errorf("device_id 错: %q", p.TrackerID)
	}
	if p.Source != "overland" {
		t.Errorf("source 错: %q", p.Source)
	}
	if len(p.MotionData) == 0 {
		t.Errorf("motion_data 应有内容")
	}
	var raw map[string]any
	if err := json.Unmarshal(p.RawData, &raw); err != nil {
		t.Fatalf("raw_data 应为 JSON: %v", err)
	}
	extra, ok := raw["overland_extra"].(map[string]any)
	if !ok || extra["current"] == nil || extra["trip"] == nil {
		t.Fatalf("raw_data 应保留 current/trip: %v", raw)
	}
}

func TestMapOverlandBatch_BadCoord(t *testing.T) {
	// 单条坐标缺失，应被跳过但整体不报错
	bad := `{"locations":[{"type":"Feature","geometry":{"type":"Point","coordinates":[]},"properties":{"timestamp":"2024-01-02T03:04:05Z"}}]}`
	ps, err := ingest.MapOverlandBatch([]byte(bad))
	if err != nil {
		t.Fatalf("不应整体报错: %v", err)
	}
	if len(ps) != 0 {
		t.Errorf("坏 feature 应被跳过: %d", len(ps))
	}
}

func TestMapOverlandBatch_BadTimestamp(t *testing.T) {
	bad := `{"locations":[{"type":"Feature","geometry":{"type":"Point","coordinates":[1,2]},"properties":{"timestamp":"not-a-time"}}]}`
	ps, _ := ingest.MapOverlandBatch([]byte(bad))
	if len(ps) != 0 {
		t.Errorf("坏时间戳应被跳过")
	}
}

func TestMapOverlandBatch_InvalidJSON(t *testing.T) {
	if _, err := ingest.MapOverlandBatch([]byte(`{bad`)); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
