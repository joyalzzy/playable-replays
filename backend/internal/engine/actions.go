package engine

import (
	"fmt"
	"math"
	"slices"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func (e *Engine) validateAction(action model.Action, controlled model.Unit) error {
	if !slices.Contains(e.session.LegalActions, action.Type) {
		return fmt.Errorf("%w: unknown action %q", ErrIllegalAction, action.Type)
	}
	if action.Type == "move" && action.Target == nil {
		return fmt.Errorf("%w: move requires a target", ErrIllegalAction)
	}
	if action.Target != nil {
		if action.Type != "move" && action.Type != "dodge" {
			return fmt.Errorf("%w: %s does not accept a target", ErrIllegalAction, action.Type)
		}
		if !pointFinite(*action.Target) {
			return fmt.Errorf("%w: target must contain finite coordinates", ErrIllegalAction)
		}
		if !pointInBounds(*action.Target) {
			return fmt.Errorf("%w: target is outside the map", ErrIllegalAction)
		}
		if distance(controlled.Position, *action.Target) > controlled.MoveRange+1e-9 {
			return fmt.Errorf(
				"%w: target exceeds %s movement limit of %.1f units per frame",
				ErrIllegalAction, controlled.Class, controlled.MoveRange,
			)
		}
	}
	return nil
}

func (e *Engine) resolveUser(unit *model.Unit, action model.Action) turnEffects {
	effects := turnEffects{}
	message := fmt.Sprintf("You committed to %s.", action.Type)
	switch action.Type {
	case "move":
		d := distance(unit.Position, *action.Target)
		unit.Position = *action.Target
		message = fmt.Sprintf("You moved %.1f units within the %s limit of %.1f.", d, unit.Class, unit.MoveRange)
	case "hold":
		unit.HP = min(unit.MaxHP, unit.HP+5)
		message = "You held position and recovered 5 health."
	case "contest":
		target := e.nearestEnemy(*unit)
		if target != nil && distance(unit.Position, target.Position) <= unit.AttackRange && unit.Cooldown == 0 {
			damage := 16 + e.rng.Intn(9)
			target.HP = max(0, target.HP-damage)
			target.Alive = target.HP > 0
			unit.Cooldown = 2
			message = fmt.Sprintf("You contested and dealt %d damage to %s.", damage, target.ID)
		} else {
			message = "You contested, but no enemy was available in attack range."
		}
	case "retreat":
		before := unit.Position
		unit.Position = moveToward(unit.Position, model.Point{X: 8, Y: 50}, unit.MoveRange)
		unit.HP = min(unit.MaxHP, unit.HP+3)
		message = fmt.Sprintf("You retreated %.1f units and recovered 3 health.", distance(before, unit.Position))
	case "dodge":
		before := unit.Position
		if action.Target != nil {
			unit.Position = *action.Target
		} else {
			unit.Position = e.automaticDodgeTarget(*unit)
		}
		effects.dodgeActive = true
		message = fmt.Sprintf("You repositioned %.1f units to dodge incoming skillshots.", distance(before, unit.Position))
	case "outplay":
		target := e.nearestEnemy(*unit)
		if target != nil && distance(unit.Position, target.Position) <= unit.AttackRange && unit.Cooldown == 0 {
			damage := 16 + int(math.Round(unit.MoveRange))
			target.HP = max(0, target.HP-damage)
			target.Alive = target.HP > 0
			unit.Cooldown = 2
			effects.outplaySucceeded = true
			message = fmt.Sprintf("You outplayed %s and dealt %d damage.", target.ID, damage)
		} else if unit.Cooldown > 0 {
			message = "The outplay was unavailable because your ability was on cooldown."
		} else {
			message = "The outplay was unavailable because no enemy was in attack range."
		}
	}
	e.session.Log = append(e.session.Log, model.LogEntry{
		Turn: e.session.Turn, Actor: "user", Action: action.Type, Message: message,
	})
	return effects
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

func (e *Engine) referenceAction(turn int) model.Action {
	sequence := []string{"move", "outplay", "dodge", "contest", "retreat"}
	action := model.Action{Type: sequence[turn%len(sequence)]}
	if action.Type == "move" {
		if controlled := e.unit(e.moment.ControlledUnitID); controlled != nil {
			target := moveToward(controlled.Position, model.Point{X: 62, Y: 48}, controlled.MoveRange)
			action.Target = &target
		}
	}
	return action
}
