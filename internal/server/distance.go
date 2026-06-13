package server

import (
	"math"

	"geokeep/internal/model"
)

// totalDistanceMeters Haversine 公式累加相邻点距离，单位米。
// 用于 /api/v1/stats/summary 的 distance_m 字段。
func totalDistanceMeters(pts []model.Point) float64 {
	const R = 6371000.0
	if len(pts) < 2 {
		return 0
	}
	total := 0.0
	for i := 1; i < len(pts); i++ {
		la1 := pts[i-1].Latitude * math.Pi / 180
		la2 := pts[i].Latitude * math.Pi / 180
		dla := la2 - la1
		dlo := (pts[i].Longitude - pts[i-1].Longitude) * math.Pi / 180
		a := math.Sin(dla/2)*math.Sin(dla/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dlo/2)*math.Sin(dlo/2)
		c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
		total += R * c
	}
	return total
}
