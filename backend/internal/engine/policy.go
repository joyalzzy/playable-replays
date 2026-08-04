package engine

import (
	"context"
	"fmt"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func (e *Engine) resolvePolicy(ctx context.Context, controlled *model.Unit, effects *turnEffects) {
	suggestions, modelUsed, fallback := e.modelSuggestions(ctx)
	e.resolveAIPositions(controlled, suggestions)
	e.resolveOpponentCombat(controlled, effects)

	message := "The deterministic opponent policy responded; allied units held position."
	action := "respond"
	if fallback {
		action = "fallback"
		message = "The position model response was unusable; deterministic opponent movement was applied and allied units held position."
	} else if modelUsed {
		action = "model-respond"
		message = "The position model response was applied to eligible non-player units; authoritative class movement limits were applied."
	}
	e.session.Log = append(e.session.Log, model.LogEntry{
		Turn: e.session.Turn, Actor: "policy", Action: action, Message: message,
	})
}

func (e *Engine) resolveAIPositions(controlled *model.Unit, suggestions map[string]model.Point) {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.ID == controlled.ID {
			continue
		}
		if desired, ok := suggestions[unit.ID]; ok {
			unit.Position = moveToward(unit.Position, desired, unit.MoveRange)
			continue
		}
		if unit.Team != controlled.Team && distance(unit.Position, controlled.Position) > unit.AttackRange {
			unit.Position = moveToward(unit.Position, controlled.Position, unit.MoveRange)
		}
	}
}

func (e *Engine) resolveOpponentCombat(controlled *model.Unit, effects *turnEffects) {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.Team == controlled.Team {
			continue
		}
		if distance(unit.Position, controlled.Position) > unit.AttackRange || unit.Cooldown != 0 {
			continue
		}

		unit.Cooldown = 2
		if effects.dodgeActive {
			effects.dodgeEvaded = true
			e.session.Log = append(e.session.Log, model.LogEntry{
				Turn: e.session.Turn, Actor: "policy", Action: "skillshot-dodged",
				Message: fmt.Sprintf("You dodged %s's skillshot.", unit.ID),
			})
			continue
		}
		damage := 8 + e.rng.Intn(8)
		if effects.outplaySucceeded {
			damage = (damage + 1) / 2
		}
		controlled.HP = max(0, controlled.HP-damage)
		controlled.Alive = controlled.HP > 0
		e.session.Log = append(e.session.Log, model.LogEntry{
			Turn: e.session.Turn, Actor: "policy", Action: "skillshot",
			Message: fmt.Sprintf("%s hit you with a skillshot for %d damage.", unit.ID, damage),
		})
	}
}
