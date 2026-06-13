package exporter_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"geokeep/internal/exporter"
	"geokeep/internal/model"
)

func samplePoints() []model.Point {
	alt := 35.0
	acc := 5
	return []model.Point{
		{Timestamp: 1704067200, Latitude: 1.0, Longitude: 2.0, Altitude: &alt, Accuracy: &acc, Velocity: "10", TrackerID: "X", Source: "owntracks"},
		{Timestamp: 1706659200, Latitude: 1.1, Longitude: 2.1, TrackerID: "X", Source: "owntracks"}, // 2024-01-31 -> 2024-01
	}
}

func TestGeoJSONWriter(t *testing.T) {
	w, _ := exporter.NewWriter(exporter.FormatGeoJSON)
	var buf bytes.Buffer
	if err := w.Open(&buf); err != nil {
		t.Fatal(err)
	}
	for _, p := range samplePoints() {
		if err := w.Write(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("产出非合法 JSON: %v\n%s", err, buf.String())
	}
	feats, ok := out["features"].([]any)
	if !ok || len(feats) != 2 {
		t.Fatalf("features 应为 2 条: %v", out)
	}
}

func TestGPXWriter(t *testing.T) {
	w, _ := exporter.NewWriter(exporter.FormatGPX)
	var buf bytes.Buffer
	_ = w.Open(&buf)
	for _, p := range samplePoints() {
		_ = w.Write(p)
	}
	_ = w.Close()
	s := buf.String()
	if !strings.Contains(s, "<gpx") || !strings.Contains(s, "</gpx>") {
		t.Fatalf("缺 gpx 标签:\n%s", s)
	}
	if !strings.Contains(s, "<trkpt") {
		t.Fatalf("缺 trkpt")
	}
}

func TestOwnTracksJSONWriter_Roundtrip(t *testing.T) {
	w, _ := exporter.NewWriter(exporter.FormatOwnTracksJSON)
	var buf bytes.Buffer
	_ = w.Open(&buf)
	for _, p := range samplePoints() {
		_ = w.Write(p)
	}
	_ = w.Close()
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 行: %d", len(lines))
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
		t.Fatalf("非 JSON: %v", err)
	}
	if obj["_type"] != "location" || obj["tid"] != "X" {
		t.Fatalf("字段错: %v", obj)
	}
}

func TestDawarichV2Writer_TarLayout(t *testing.T) {
	w, _ := exporter.NewWriter(exporter.FormatDawarichV2)
	var buf bytes.Buffer
	_ = w.Open(&buf)
	for _, p := range samplePoints() {
		_ = w.Write(p)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var sawJSONL, sawManifest bool
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.HasPrefix(h.Name, "points/") && strings.HasSuffix(h.Name, ".jsonl"):
			sawJSONL = true
		case h.Name == "MANIFEST.json":
			sawManifest = true
		}
	}
	if !sawJSONL || !sawManifest {
		t.Fatalf("缺关键条目: jsonl=%v manifest=%v", sawJSONL, sawManifest)
	}
}

func TestUnsupportedExportFormat(t *testing.T) {
	if _, err := exporter.NewWriter("???"); err == nil {
		t.Fatal("应拒绝未知格式")
	}
}
