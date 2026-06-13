package importer_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"

	"geokeep/internal/importer"
	"geokeep/internal/model"
)

func collect(t *testing.T, parser importer.Parser, input string) []*model.Point {
	t.Helper()
	var got []*model.Point
	err := parser.Parse(context.Background(), strings.NewReader(input), func(p *model.Point) error {
		got = append(got, p)
		return nil
	})
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	return got
}

func TestOwnTracksRec(t *testing.T) {
	p, _ := importer.NewParser(importer.FormatOwntracksRec)
	input := "2024-01-02T03:04:05Z\t*\t{\"_type\":\"location\",\"lat\":1.0,\"lon\":2.0,\"tst\":100,\"tid\":\"X\"}\n" +
		"# comment\n" +
		"2024-01-02T03:04:06Z\t*\t{\"_type\":\"location\",\"lat\":1.1,\"lon\":2.1,\"tst\":101,\"tid\":\"Y\"}\n"
	pts := collect(t, p, input)
	if len(pts) != 2 {
		t.Fatalf("应有 2 条: %d", len(pts))
	}
	if pts[0].Timestamp != 100 || pts[1].Timestamp != 101 {
		t.Fatalf("时间戳错: %+v", pts)
	}
}

func TestDawarichV1(t *testing.T) {
	p, _ := importer.NewParser(importer.FormatDawarichV1)
	input := `{
	  "users": [{"id":1}],
	  "points": [
	    {"latitude":1.1, "longitude":2.2, "timestamp":1000, "tracker_id":"X"},
	    {"latitude":1.2, "longitude":2.3, "timestamp":1001}
	  ],
	  "trips": []
	}`
	pts := collect(t, p, input)
	if len(pts) != 2 {
		t.Fatalf("应有 2 条: %d", len(pts))
	}
	if pts[0].TrackerID != "X" {
		t.Fatalf("tracker_id 错")
	}
}

func TestGeoJSON_FeatureCollection(t *testing.T) {
	p, _ := importer.NewParser(importer.FormatGeoJSON)
	input := `{
	  "type":"FeatureCollection",
	  "features":[
	    {"type":"Feature","geometry":{"type":"Point","coordinates":[2.0,1.0]},"properties":{"timestamp":"2024-01-02T03:04:05Z"}},
	    {"type":"Feature","geometry":{"type":"Point","coordinates":[3.0,1.1]},"properties":{"ts":1000}}
	  ]
	}`
	pts := collect(t, p, input)
	if len(pts) != 2 {
		t.Fatalf("应有 2 条: %d", len(pts))
	}
}

func TestGeoJSON_RejectLineString(t *testing.T) {
	p, _ := importer.NewParser(importer.FormatGeoJSON)
	input := `{"type":"FeatureCollection","features":[{"type":"Feature","geometry":{"type":"LineString","coordinates":[[1,2],[3,4]]},"properties":{}}]}`
	err := p.Parse(context.Background(), strings.NewReader(input), func(*model.Point) error { return nil })
	if err == nil {
		t.Fatal("LineString 应被拒绝")
	}
}

func TestGPX_Roundtrip(t *testing.T) {
	p, _ := importer.NewParser(importer.FormatGPX)
	input := `<?xml version="1.0"?>
<gpx xmlns="http://www.topografix.com/GPX/1/1" version="1.1">
  <trk><name>t</name><trkseg>
    <trkpt lat="1.0" lon="2.0"><ele>10</ele><time>2024-01-02T03:04:05Z</time></trkpt>
    <trkpt lat="1.1" lon="2.1"><ele>11</ele><time>2024-01-02T03:04:06Z</time></trkpt>
  </trkseg></trk>
</gpx>`
	pts := collect(t, p, input)
	if len(pts) != 2 {
		t.Fatalf("应有 2 条: %d", len(pts))
	}
	if pts[0].Latitude != 1.0 || pts[0].Longitude != 2.0 {
		t.Fatalf("coord 错")
	}
	if pts[0].Altitude == nil || *pts[0].Altitude != 10 {
		t.Fatalf("alt 错")
	}
}

func TestDawarichV2_TarGz(t *testing.T) {
	// 手工拼一个 .tar.gz：含 points/2024-01.jsonl
	jsonl := `{"latitude":1.0,"longitude":2.0,"timestamp":1000}` + "\n" +
		`{"latitude":1.1,"longitude":2.1,"timestamp":1001}` + "\n"

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	_ = tw.WriteHeader(&tar.Header{Name: "points/2024-01.jsonl", Mode: 0o600, Size: int64(len(jsonl))})
	_, _ = tw.Write([]byte(jsonl))
	// 另一个无关条目
	other := "ignored"
	_ = tw.WriteHeader(&tar.Header{Name: "tracks/x.json", Mode: 0o600, Size: int64(len(other))})
	_, _ = tw.Write([]byte(other))
	_ = tw.Close()

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write(tarBuf.Bytes())
	_ = gw.Close()

	p, _ := importer.NewParser(importer.FormatDawarichV2)
	var got []*model.Point
	err := p.Parse(context.Background(), bytes.NewReader(gzBuf.Bytes()), func(p *model.Point) error {
		got = append(got, p)
		return nil
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应有 2 条: %d", len(got))
	}
}

func TestDawarichV2_NoPointsEntry(t *testing.T) {
	// 空 tar.gz
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	_ = tw.Close()
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write(tarBuf.Bytes())
	_ = gw.Close()
	p, _ := importer.NewParser(importer.FormatDawarichV2)
	err := p.Parse(context.Background(), bytes.NewReader(gzBuf.Bytes()), func(*model.Point) error { return nil })
	if err == nil {
		t.Fatal("空归档应报错（不静默成功）")
	}
}

func TestDetectFormat(t *testing.T) {
	cases := map[string]string{
		"a.gpx":     importer.FormatGPX,
		"b.geojson": importer.FormatGeoJSON,
		"c.json":    importer.FormatGeoJSON,
		"d.rec":     importer.FormatOwntracksRec,
		"e.tar.gz":  importer.FormatDawarichV2,
		"f.tgz":     importer.FormatDawarichV2,
		"g.zip":     importer.FormatDawarichV2,
		"h.txt":     "",
	}
	for n, want := range cases {
		if got := importer.DetectFormat(n); got != want {
			t.Errorf("DetectFormat(%q) = %q, want %q", n, got, want)
		}
	}
}

func TestUnsupportedFormat(t *testing.T) {
	if _, err := importer.NewParser("???"); err == nil {
		t.Fatal("应拒绝未知格式")
	}
}
