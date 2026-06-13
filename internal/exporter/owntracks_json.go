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
	if p.Course != nil {
		obj["cog"] = *p.Course
	}
	if p.Battery != nil {
		obj["batt"] = *p.Battery
	}
	if p.BatteryStatus != nil {
		obj["bs"] = *p.BatteryStatus
	}
	if p.Connection != nil {
		obj["conn"] = reverseConnection(*p.Connection)
	}
	if p.Trigger != nil {
		obj["t"] = reverseTrigger(*p.Trigger)
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
	if xs := stringArray(p.InRegions); len(xs) > 0 {
		obj["inregions"] = xs
	}
	if xs := stringArray(p.InRIDs); len(xs) > 0 {
		obj["inrids"] = xs
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

func reverseConnection(v int) string {
	switch v {
	case 0:
		return "w"
	case 1:
		return "o"
	case 2:
		return "m"
	default:
		return ""
	}
}

func reverseTrigger(v int) string {
	switch v {
	case 0:
		return "p"
	case 1:
		return "c"
	case 2:
		return "b"
	case 3:
		return "r"
	case 4:
		return "u"
	case 5:
		return "t"
	case 6:
		return "v"
	default:
		return ""
	}
}

func stringArray(raw string) []string {
	if raw == "" {
		return nil
	}
	var xs []string
	if err := json.Unmarshal([]byte(raw), &xs); err != nil {
		return nil
	}
	return xs
}
