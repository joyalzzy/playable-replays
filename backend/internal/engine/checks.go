package engine

import (
	"math"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func pointInBounds(point model.Point) bool {
	return point.X >= MapMin && point.X <= MapMax && point.Y >= MapMin && point.Y <= MapMax
}
func (e *Engine) nearestEnemy(from model.Unit) *model.Unit {
	var nearest *model.Unit
	best := math.MaxFloat64
	for i := range e.session.Units {
		candidate := &e.session.Units[i]
		d := distance(from.Position, candidate.Position)
		if candidate.Alive && candidate.Team != from.Team && d < best {
			nearest, best = candidate, d
		}
	}
	return nearest
}

func (e *Engine) automaticDodgeTarget(unit model.Unit) model.Point {
	enemy := e.nearestEnemy(unit)
	if enemy == nil {
		return moveToward(unit.Position, model.Point{X: 50, Y: 50}, unit.MoveRange)
	}
	dx := enemy.Position.X - unit.Position.X
	dy := enemy.Position.Y - unit.Position.Y
	d := math.Hypot(dx, dy)
	if d == 0 {
		dx, dy, d = 1, 0, 1
	}
	first := model.Point{X: unit.Position.X - dy/d*unit.MoveRange, Y: unit.Position.Y + dx/d*unit.MoveRange}
	second := model.Point{X: unit.Position.X + dy/d*unit.MoveRange, Y: unit.Position.Y - dx/d*unit.MoveRange}
	first = clampPoint(first)
	second = clampPoint(second)
	if distance(unit.Position, second) > distance(unit.Position, first) {
		return second
	}
	return first
}
