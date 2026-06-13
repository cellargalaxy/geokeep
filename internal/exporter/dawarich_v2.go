package exporter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"geokeep/internal/model"
)

// dawarichV2Writer 产出 .tar.gz：内含 points/YYYY-MM.jsonl 月份分片 + MANIFEST.json。
// 与 dawarich v2 archive 布局对齐（manifest 文件名待与官方对拍）。
type dawarichV2Writer struct {
	dest    io.Writer
	gz      *gzip.Writer
	tw      *tar.Writer
	monthly map[string]*bytes.Buffer // key: "2024-01" → 累积 JSONL
}

func (d *dawarichV2Writer) Extension() string { return "tar.gz" }

func (d *dawarichV2Writer) Open(w io.Writer) error {
	d.dest = w
	d.gz = gzip.NewWriter(w)
	d.tw = tar.NewWriter(d.gz)
	d.monthly = make(map[string]*bytes.Buffer)
	return nil
}

func (d *dawarichV2Writer) Write(p model.Point) error {
	t := time.Unix(p.Timestamp, 0).UTC()
	key := fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
	buf, ok := d.monthly[key]
	if !ok {
		buf = &bytes.Buffer{}
		d.monthly[key] = buf
	}
	// 输出字段命名对齐 dawarich points 表
	obj := map[string]any{
		"latitude":  p.Latitude,
		"longitude": p.Longitude,
		"timestamp": p.Timestamp,
	}
	if p.Altitude != nil {
		obj["altitude"] = *p.Altitude
	}
	if p.Velocity != "" {
		obj["velocity"] = p.Velocity
	}
	if p.Accuracy != nil {
		obj["accuracy"] = *p.Accuracy
	}
	if p.Battery != nil {
		obj["battery"] = *p.Battery
	}
	if p.TrackerID != "" {
		obj["tracker_id"] = p.TrackerID
	}
	if p.Topic != "" {
		obj["topic"] = p.Topic
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	buf.Write(b)
	buf.WriteByte('\n')
	return nil
}

func (d *dawarichV2Writer) Close() error {
	for month, buf := range d.monthly {
		name := fmt.Sprintf("points/%s.jsonl", month)
		if err := d.tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o600, Size: int64(buf.Len()),
		}); err != nil {
			return err
		}
		if _, err := d.tw.Write(buf.Bytes()); err != nil {
			return err
		}
	}
	manifest := map[string]any{
		"exporter": "geokeep",
		"format":   "dawarich_v2",
		"missing_columns": []string{
			"country", "city", "geodata", "visit_id", "track_id", "external_track_id",
		},
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	if err := d.tw.WriteHeader(&tar.Header{Name: "MANIFEST.json", Mode: 0o600, Size: int64(len(mb))}); err != nil {
		return err
	}
	if _, err := d.tw.Write(mb); err != nil {
		return err
	}
	if err := d.tw.Close(); err != nil {
		return err
	}
	return d.gz.Close()
}
