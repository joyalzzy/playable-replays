package engine

import (
	"math"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	MapMin = 0.0
	MapMax = 100.0
)

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
