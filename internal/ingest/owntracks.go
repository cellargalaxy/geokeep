// Package ingest 将外部 payload（OwnTracks / Overland）映射为内部 model.Point。
// 字段定义来源：
//   - OwnTracks: https://owntracks.org/booklet/tech/json/ §location/transition/...
//   - Overland: https://github.com/aaronpk/Overland-iOS README
package ingest

import (
	"encoding/json"
	"errors"
	"strconv"

	"geokeep/internal/model"
)

// OwnTracksMessage 用于探测 _type 字段，再分支到 location 等。
type OwnTracksMessage struct {
	Type string `json:"_type"`
}

// OwnTracksLocation 对齐 _type=location 已知字段。
// 缺省字段保持 *T 形式，便于「未上报 vs 上报 0」区分。
type OwnTracksLocation struct {
	Type      string   `json:"_type"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	Tst       int64    `json:"tst"`
	Acc       *int     `json:"acc,omitempty"`
	Alt       *int     `json:"alt,omitempty"`
	Vac       *int     `json:"vac,omitempty"`
	Vel       *int     `json:"vel,omitempty"`
	Cog       *int     `json:"cog,omitempty"`
	Batt      *int     `json:"batt,omitempty"`
	BS        *int     `json:"bs,omitempty"`
	Conn      string   `json:"conn,omitempty"`
	Trig      string   `json:"t,omitempty"`
	Tid       string   `json:"tid,omitempty"`
	Topic     string   `json:"topic,omitempty"`
	SSID      string   `json:"SSID,omitempty"`
	BSSID     string   `json:"BSSID,omitempty"`
	InRegions []string `json:"inregions,omitempty"`
	InRids    []string `json:"inrids,omitempty"`
}

// ErrOwnTracksNotLocation 非 location 类型；上层应仅留 raw_data 不落点表。
var ErrOwnTracksNotLocation = errors.New("owntracks: 非 location 类型")

// MapOwnTracksLocation 把单条 OwnTracks location payload 映射为 Point。
// raw 必须是收到的原始字节，将完整存入 point.RawData。
func MapOwnTracksLocation(raw []byte) (*model.Point, error) {
	var msg OwnTracksMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	if msg.Type != "location" {
		return nil, ErrOwnTracksNotLocation
	}
	var loc OwnTracksLocation
	if err := json.Unmarshal(raw, &loc); err != nil {
		return nil, err
	}
	p := &model.Point{
		Timestamp: loc.Tst,
		Latitude:  loc.Lat,
		Longitude: loc.Lon,
		TrackerID: loc.Tid,
		Topic:     loc.Topic,
		SSID:      loc.SSID,
		BSSID:     loc.BSSID,
		RawData:   append([]byte(nil), raw...),
		Source:    "owntracks",
	}
	if loc.Alt != nil {
		v := float64(*loc.Alt)
		p.Altitude = &v
	}
	if loc.Acc != nil {
		v := *loc.Acc
		p.Accuracy = &v
	}
	if loc.Vac != nil {
		v := *loc.Vac
		p.VerticalAccuracy = &v
	}
	if loc.Vel != nil {
		p.Velocity = strconv.Itoa(*loc.Vel)
	}
	if loc.Cog != nil {
		v := float64(*loc.Cog)
		p.Course = &v
	}
	if loc.Batt != nil {
		v := *loc.Batt
		p.Battery = &v
	}
	if loc.BS != nil {
		v := *loc.BS
		p.BatteryStatus = &v
	}
	if loc.Conn != "" {
		v := mapConnection(loc.Conn)
		p.Connection = &v
	}
	if loc.Trig != "" {
		v := mapTrigger(loc.Trig)
		p.Trigger = &v
	}
	if len(loc.InRegions) > 0 {
		b, _ := json.Marshal(loc.InRegions)
		p.InRegions = string(b)
	}
	if len(loc.InRids) > 0 {
		b, _ := json.Marshal(loc.InRids)
		p.InRIDs = string(b)
	}
	return p, nil
}

// mapConnection 将 OwnTracks conn 字符串映射为整数（与 dawarich 枚举的精确映射待确认）。
// w(WiFi)=0  o(offline)=1  m(mobile)=2。
func mapConnection(s string) int {
	switch s {
	case "w":
		return 0
	case "o":
		return 1
	case "m":
		return 2
	}
	return -1
}

// mapTrigger 将 OwnTracks t 字段单字符映射为整数。
// p(ping)=0 c(region)=1 b(beacon)=2 r(cmd response)=3 u(manual)=4 t(timer)=5 v(frequent locations)=6。
func mapTrigger(s string) int {
	switch s {
	case "p":
		return 0
	case "c":
		return 1
	case "b":
		return 2
	case "r":
		return 3
	case "u":
		return 4
	case "t":
		return 5
	case "v":
		return 6
	}
	return -1
}
