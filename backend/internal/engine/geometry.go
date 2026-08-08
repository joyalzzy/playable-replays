package engine

import (
	"math"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	MapMin = 0.0
	MapMax = 100.0
)

var fullMapTurrets = []model.Turret{
	{ID: "blue-top-turret", Team: "blue", Lane: "top", Position: model.Point{X: 8.5, Y: 59}, HP: 3000, MaxHP: 3000, Alive: true},
	{ID: "blue-middle-turret", Team: "blue", Lane: "middle", Position: model.Point{X: 29, Y: 70.5}, HP: 3000, MaxHP: 3000, Alive: true},
	{ID: "blue-bottom-turret", Team: "blue", Lane: "bottom", Position: model.Point{X: 40.7, Y: 91.5}, HP: 3000, MaxHP: 3000, Alive: true},
	{ID: "red-top-turret", Team: "red", Lane: "top", Position: model.Point{X: 59.7, Y: 7.5}, HP: 3000, MaxHP: 3000, Alive: true},
	{ID: "red-middle-turret", Team: "red", Lane: "middle", Position: model.Point{X: 71.4, Y: 28.3}, HP: 3000, MaxHP: 3000, Alive: true},
	{ID: "red-bottom-turret", Team: "red", Lane: "bottom", Position: model.Point{X: 92, Y: 40}, HP: 3000, MaxHP: 3000, Alive: true},
}

func canonicalTurrets() []model.Turret {
	return append([]model.Turret(nil), fullMapTurrets...)
}

func distance(a, b model.Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func moveToward(from, to model.Point, limit float64) model.Point {
	d := distance(from, to)
	if d == 0 || d <= limit {
		return to
	}
	return model.Point{
		X: from.X + (to.X-from.X)*limit/d,
		Y: from.Y + (to.Y-from.Y)*limit/d,
	}
}

func pointFinite(point model.Point) bool {
	return !math.IsNaN(point.X) && !math.IsInf(point.X, 0) &&
		!math.IsNaN(point.Y) && !math.IsInf(point.Y, 0)
}

func pointInBounds(point model.Point) bool {
	return point.X >= MapMin && point.X <= MapMax && point.Y >= MapMin && point.Y <= MapMax
}

func clampPoint(point model.Point) model.Point {
	return model.Point{X: clamp(point.X, MapMin, MapMax), Y: clamp(point.Y, MapMin, MapMax)}
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
