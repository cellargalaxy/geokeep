package exporter

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"geokeep/internal/model"
)

// dawarichV2Writer 产出 .tar.gz：内含 points/YYYY-MM.jsonl 月份分片 + MANIFEST.json。
// 与 dawarich v2 archive 布局对齐（manifest 文件名待与官方对拍）。
type dawarichV2Writer struct {
	dest    io.Writer
	gz      *gzip.Writer
	tw      *tar.Writer
	monthly map[string]*os.File // key: "2024-01" → 临时文件
}

func (d *dawarichV2Writer) Extension() string { return "tar.gz" }

func (d *dawarichV2Writer) Open(w io.Writer) error {
	d.dest = w
	d.gz = gzip.NewWriter(w)
	d.tw = tar.NewWriter(d.gz)
	d.monthly = make(map[string]*os.File)
	return nil
}

func (d *dawarichV2Writer) Write(p model.Point) error {
	t := time.Unix(p.Timestamp, 0).UTC()
	key := fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
	f, ok := d.monthly[key]
	if !ok {
		var err error
		f, err = os.CreateTemp("", "geokeep-export-*.jsonl")
		if err != nil {
			return err
		}
		d.monthly[key] = f
	}
	b, err := json.Marshal(dawarichObject(p))
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	if _, err := f.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

func (d *dawarichV2Writer) Close() error {
	defer func() {
		for _, f := range d.monthly {
			f.Close()
			os.Remove(f.Name())
		}
	}()

	for month, f := range d.monthly {
		st, err := f.Stat()
		if err != nil {
			return err
		}
		if _, err := f.Seek(0, 0); err != nil {
			return err
		}
		name := fmt.Sprintf("points/%s.jsonl", month)
		if err := d.tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o600, Size: st.Size(),
		}); err != nil {
			return err
		}
		if _, err := io.Copy(d.tw, f); err != nil {
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
