package exporter

import (
	"encoding/json"
	"strings"

	"geokeep/internal/model"
)

func dawarichObject(p model.Point) map[string]any {
	obj := map[string]any{}
	fromDawarich := strings.HasPrefix(p.Source, "import:dawarich")
	if fromDawarich && len(p.RawData) > 0 {
		var rawObj map[string]any
		if err := json.Unmarshal(p.RawData, &rawObj); err == nil {
			obj = rawObj
		}
	}

	obj["latitude"] = p.Latitude
	obj["longitude"] = p.Longitude
	obj["timestamp"] = p.Timestamp
	putPtr(obj, "altitude", p.Altitude)
	putPtr(obj, "accuracy", p.Accuracy)
	putPtr(obj, "vertical_accuracy", p.VerticalAccuracy)
	if p.Velocity != "" {
		obj["velocity"] = p.Velocity
	}
	putPtr(obj, "course", p.Course)
	putPtr(obj, "course_accuracy", p.CourseAccuracy)
	putPtr(obj, "battery", p.Battery)
	putPtr(obj, "battery_status", p.BatteryStatus)
	putPtr(obj, "connection", p.Connection)
	if p.SSID != "" {
		obj["ssid"] = p.SSID
	}
	if p.BSSID != "" {
		obj["bssid"] = p.BSSID
	}
	putPtr(obj, "trigger", p.Trigger)
	if p.TrackerID != "" {
		obj["tracker_id"] = p.TrackerID
	}
	if p.Topic != "" {
		obj["topic"] = p.Topic
	}
	if p.InRegions != "" {
		obj["in_regions"] = rawJSONOrString([]byte(p.InRegions))
	}
	if p.InRIDs != "" {
		obj["inrids"] = rawJSONOrString([]byte(p.InRIDs))
	}
	if len(p.MotionData) > 0 {
		obj["motion_data"] = rawJSONOrString(p.MotionData)
	}
	if len(p.RawData) > 0 && !fromDawarich {
		obj["raw_data"] = rawJSONOrString(p.RawData)
	}
	return obj
}

func putPtr[T any](m map[string]any, key string, v *T) {
	if v != nil {
		m[key] = *v
	}
}

func rawJSONOrString(raw []byte) any {
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		return v
	}
	return string(raw)
}
