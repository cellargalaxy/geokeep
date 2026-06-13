package exporter

import (
	"encoding/json"
	"io"
	"strconv"

	"geokeep/internal/model"
)

// owntracksJSONWriter 输出行式 JSONL，每行一个 _type=location 对象，
// 字段集合对齐 OwnTracks 上报 payload，便于喂回 OwnTracks Recorder。
type owntracksJSONWriter struct{ w io.Writer }

func (o *owntracksJSONWriter) Extension() string { return "jsonl" }

func (o *owntracksJSONWriter) Open(w io.Writer) error {
	o.w = w
	return nil
}

func (o *owntracksJSONWriter) Write(p model.Point) error {
	obj := map[string]any{
		"_type": "location",
		"lat":   p.Latitude,
		"lon":   p.Longitude,
		"tst":   p.Timestamp,
	}
	if p.Accuracy != nil {
		obj["acc"] = *p.Accuracy
	}
	if p.Altitude != nil {
		obj["alt"] = int(*p.Altitude)
	}
	if p.VerticalAccuracy != nil {
		obj["vac"] = *p.VerticalAccuracy
	}
	if p.Velocity != "" {
		if n, err := strconv.Atoi(p.Velocity); err == nil {
			obj["vel"] = n
		}
	}
	if p.Battery != nil {
		obj["batt"] = *p.Battery
	}
	if p.BatteryStatus != nil {
		obj["bs"] = *p.BatteryStatus
	}
	if p.TrackerID != "" {
		obj["tid"] = p.TrackerID
	}
	if p.Topic != "" {
		obj["topic"] = p.Topic
	}
	if p.SSID != "" {
		obj["SSID"] = p.SSID
	}
	if p.BSSID != "" {
		obj["BSSID"] = p.BSSID
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if _, err := o.w.Write(b); err != nil {
		return err
	}
	_, err = o.w.Write([]byte("\n"))
	return err
}

func (o *owntracksJSONWriter) Close() error { return nil }
