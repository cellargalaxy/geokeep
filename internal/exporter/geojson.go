package exporter

import (
	"encoding/json"
	"errors"
	"io"

	"geokeep/internal/model"
)

// geoJSONWriter 产出 FeatureCollection<Point>，properties 平铺常用字段。
type geoJSONWriter struct {
	w      io.Writer
	first  bool
	opened bool
}

func (g *geoJSONWriter) Extension() string { return "geojson" }

func (g *geoJSONWriter) Open(w io.Writer) error {
	g.w = w
	g.first = true
	g.opened = true
	_, err := io.WriteString(g.w, `{"type":"FeatureCollection","features":[`)
	return err
}

func (g *geoJSONWriter) Write(p model.Point) error {
	if !g.opened {
		return errors.New("writer 未初始化")
	}
	if !g.first {
		if _, err := io.WriteString(g.w, ","); err != nil {
			return err
		}
	}
	g.first = false
	props := map[string]any{
		"ts":         p.Timestamp,
		"source":     p.Source,
		"tracker_id": p.TrackerID,
	}
	if p.Altitude != nil {
		props["altitude"] = *p.Altitude
	}
	if p.Velocity != "" {
		props["velocity"] = p.Velocity
	}
	if p.Accuracy != nil {
		props["accuracy"] = *p.Accuracy
	}
	if p.Battery != nil {
		props["battery"] = *p.Battery
	}
	feature := map[string]any{
		"type":       "Feature",
		"geometry":   map[string]any{"type": "Point", "coordinates": []float64{p.Longitude, p.Latitude}},
		"properties": props,
	}
	b, err := json.Marshal(feature)
	if err != nil {
		return err
	}
	_, err = g.w.Write(b)
	return err
}

func (g *geoJSONWriter) Close() error {
	_, err := io.WriteString(g.w, "]}")
	return err
}
