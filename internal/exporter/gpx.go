package exporter

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"

	"geokeep/internal/model"
)

// gpxWriter 直接写 GPX 1.1 文本（GPX 体积小，不必走 gpxgo struct 模型）。
type gpxWriter struct {
	w    io.Writer
	open bool
}

func (g *gpxWriter) Extension() string { return "gpx" }

func (g *gpxWriter) Open(w io.Writer) error {
	g.w = w
	g.open = true
	_, err := io.WriteString(g.w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
		`<gpx xmlns="http://www.topografix.com/GPX/1/1" version="1.1" creator="geokeep">`+"\n"+
		"<trk><name>geokeep</name><trkseg>\n")
	return err
}

func (g *gpxWriter) Write(p model.Point) error {
	ts := time.Unix(p.Timestamp, 0).UTC().Format(time.RFC3339)
	lat := xmlEscape(fmt.Sprintf("%.7f", p.Latitude))
	lon := xmlEscape(fmt.Sprintf("%.7f", p.Longitude))
	body := fmt.Sprintf(`<trkpt lat="%s" lon="%s">`, lat, lon)
	if p.Altitude != nil {
		body += fmt.Sprintf(`<ele>%.2f</ele>`, *p.Altitude)
	}
	body += fmt.Sprintf(`<time>%s</time></trkpt>`, ts)
	_, err := io.WriteString(g.w, body+"\n")
	return err
}

func (g *gpxWriter) Close() error {
	if !g.open {
		return nil
	}
	_, err := io.WriteString(g.w, "</trkseg></trk>\n</gpx>\n")
	return err
}

func xmlEscape(s string) string {
	var b []byte
	_ = xml.EscapeText(byteWriter{&b}, []byte(s))
	return string(b)
}

type byteWriter struct{ b *[]byte }

func (w byteWriter) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}
