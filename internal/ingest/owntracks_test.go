package ingest_test

import (
	"bytes"
	"testing"

	"geokeep/internal/ingest"
)

// 官方样例（OwnTracks 文档典型 location payload）
const sampleOwnTracks = `{
  "_type": "location",
  "lat": 48.8566,
  "lon": 2.3522,
  "tst": 1693478400,
  "acc": 25,
  "alt": 35,
  "vac": 4,
  "vel": 12,
  "cog": 270,
  "batt": 85,
  "bs": 2,
  "tid": "JD",
  "topic": "owntracks/jane/phone",
  "conn": "w",
  "t": "u",
  "SSID": "Home",
  "BSSID": "00:11:22:33:44:55",
  "inregions": ["home"],
  "inrids": ["r1"]
}`

func TestMapOwnTracksLocation_AllFields(t *testing.T) {
	p, err := ingest.MapOwnTracksLocation([]byte(sampleOwnTracks))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if p.Timestamp != 1693478400 {
		t.Errorf("tst 错: %d", p.Timestamp)
	}
	if p.Latitude != 48.8566 || p.Longitude != 2.3522 {
		t.Errorf("coord 错: %v %v", p.Latitude, p.Longitude)
	}
	if p.Accuracy == nil || *p.Accuracy != 25 {
		t.Errorf("acc 错")
	}
	if p.Altitude == nil || *p.Altitude != 35 {
		t.Errorf("alt 错: %v", p.Altitude)
	}
	if p.Velocity != "12" {
		t.Errorf("vel 错: %q", p.Velocity)
	}
	if p.Battery == nil || *p.Battery != 85 {
		t.Errorf("batt 错")
	}
	if p.BatteryStatus == nil || *p.BatteryStatus != 2 {
		t.Errorf("bs 错")
	}
	if p.TrackerID != "JD" {
		t.Errorf("tid 错: %q", p.TrackerID)
	}
	if p.Topic != "owntracks/jane/phone" {
		t.Errorf("topic 错")
	}
	if p.SSID != "Home" || p.BSSID != "00:11:22:33:44:55" {
		t.Errorf("wifi 错")
	}
	if p.Source != "owntracks" {
		t.Errorf("source 错")
	}
	if !bytes.Contains(p.RawData, []byte(`"_type"`)) {
		t.Errorf("raw_data 未保留原始字节")
	}
	if p.Connection == nil || *p.Connection != 0 {
		t.Errorf("conn=w 应映射 0")
	}
	if p.Trigger == nil || *p.Trigger != 4 {
		t.Errorf("t=u 应映射 4")
	}
	if p.InRegions == "" || p.InRIDs == "" {
		t.Errorf("regions 数组未序列化")
	}
}

func TestMapOwnTracksLocation_NonLocation(t *testing.T) {
	_, err := ingest.MapOwnTracksLocation([]byte(`{"_type":"transition"}`))
	if err != ingest.ErrOwnTracksNotLocation {
		t.Fatalf("非 location 应被识别: %v", err)
	}
}

func TestMapOwnTracksLocation_MinimumFields(t *testing.T) {
	p, err := ingest.MapOwnTracksLocation([]byte(`{"_type":"location","lat":1.0,"lon":2.0,"tst":100,"tid":"X"}`))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if p.Velocity != "" || p.Battery != nil || p.Accuracy != nil {
		t.Errorf("缺省字段应为零值: vel=%q batt=%v acc=%v", p.Velocity, p.Battery, p.Accuracy)
	}
}

func TestMapOwnTracksLocation_InvalidJSON(t *testing.T) {
	if _, err := ingest.MapOwnTracksLocation([]byte(`{bad`)); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
