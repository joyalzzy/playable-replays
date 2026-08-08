package engine

import (
	"fmt"
	"math"
	"slices"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func (e *Engine) validateAction(action model.Action) error {
	if !slices.Contains(e.session.LegalActions, action.Type) {
		return fmt.Errorf("%w: unknown action %q", ErrIllegalAction, action.Type)
	}
	if action.Type == "move" && action.Target == nil {
		return fmt.Errorf("%w: move requires a target", ErrIllegalAction)
	}
	if action.Target == nil {
		return nil
	}
	if action.Type != "move" {
		return fmt.Errorf("%w: %s does not accept a target", ErrIllegalAction, action.Type)
	}
	if !pointFinite(*action.Target) {
		return fmt.Errorf("%w: target must contain finite coordinates", ErrIllegalAction)
	}
	if !pointInBounds(*action.Target) {
		return fmt.Errorf("%w: target is outside the map", ErrIllegalAction)
	}
	return nil
}

func (e *Engine) validatePlayerAction(action model.Action, targetUnitID string) error {
	if err := e.validateAction(action); err != nil {
		return err
	}
	if targetUnitID == "" {
		return nil
	}
	if action.Type != "contest" {
		return fmt.Errorf("%w: targetUnitId is only accepted for contest", ErrIllegalAction)
	}
	controlled := e.unit(e.session.ControlledUnitID)
	target := e.unit(targetUnitID)
	if controlled == nil || !controlled.Alive || target == nil || !target.Alive ||
		target.Team == controlled.Team || !target.Visible {
		return fmt.Errorf("%w: selected contest target is unavailable", ErrIllegalAction)
	}
	if distance(controlled.Position, target.Position) > controlled.AttackRange {
		return fmt.Errorf("%w: selected contest target is outside attack range", ErrIllegalAction)
	}
	if controlled.Cooldown > 1 {
		return fmt.Errorf("%w: controlled unit attack will not be ready this turn", ErrIllegalAction)
	}
	return nil
}

func (e *Engine) automaticDodgeTarget(unit model.Unit) model.Point {
	enemy := e.nearestVisibleEnemy(unit)
	if enemy == nil {
		return moveToward(unit.Position, model.Point{X: 50, Y: 50}, unit.MoveRange)
	}
	dx := enemy.Position.X - unit.Position.X
	dy := enemy.Position.Y - unit.Position.Y
	distanceToEnemy := math.Hypot(dx, dy)
	if distanceToEnemy == 0 {
		dx, dy, distanceToEnemy = 1, 0, 1
	}
	first := model.Point{
		X: unit.Position.X - dy/distanceToEnemy*unit.MoveRange,
		Y: unit.Position.Y + dx/distanceToEnemy*unit.MoveRange,
	}
	second := model.Point{
		X: unit.Position.X + dy/distanceToEnemy*unit.MoveRange,
		Y: unit.Position.Y - dx/distanceToEnemy*unit.MoveRange,
	}
	first = clampPoint(first)
	second = clampPoint(second)
	if distance(unit.Position, second) > distance(unit.Position, first) {
		return second
	}
	return first
}
