package engine

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

var ErrIllegalAction = errors.New("illegal action")

type Engine struct {
	moment  model.Moment
	session model.Session
	rng     *rand.Rand
}

func New(moment model.Moment, sessionID string) *Engine {
	e := &Engine{moment: cloneMoment(moment)}
	e.Reset(sessionID)
	return e
}

func (e *Engine) Reset(sessionID string) model.Session {
	e.rng = rand.New(rand.NewSource(e.moment.Seed))
	e.session = model.Session{
		ID:              sessionID,
		MomentID:        e.moment.ID,
		MaxTurns:        e.moment.MaxTurns,
		Status:          "active",
		Score:           0,
		WinProbability:  clamp(0.5-e.moment.Signals.ResourceAsymmetry*0.1, 0.05, 0.95),
		ReferenceAction: referenceAction(0),
		LegalActions:    []string{"move", "hold", "contest", "retreat"},
		Units:           cloneUnits(e.moment.Units),
		Log:             []model.LogEntry{},
	}
	e.applyFog()
	return e.State()
}

func (e *Engine) State() model.Session {
	state := e.session
	state.Units = cloneUnits(e.session.Units)
	state.Log = slices.Clone(e.session.Log)
	state.LegalActions = slices.Clone(e.session.LegalActions)
	return state
}

func (e *Engine) Apply(action model.Action) (model.Session, error) {
	if e.session.Status != "active" {
		return e.State(), fmt.Errorf("%w: session is complete", ErrIllegalAction)
	}
	if !slices.Contains(e.session.LegalActions, action.Type) {
		return e.State(), fmt.Errorf("%w: unknown action %q", ErrIllegalAction, action.Type)
	}
	if action.Type == "move" && action.Target == nil {
		return e.State(), fmt.Errorf("%w: move requires a target", ErrIllegalAction)
	}
	if action.Target != nil && (action.Target.X < 0 || action.Target.X > 100 || action.Target.Y < 0 || action.Target.Y > 100) {
		return e.State(), fmt.Errorf("%w: target is outside the map", ErrIllegalAction)
	}

	e.session.Turn++
	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return e.State(), fmt.Errorf("%w: controlled unit is unavailable", ErrIllegalAction)
	}
	e.resolveUser(controlled, action)
	e.resolvePolicy(controlled)
	e.tickCooldowns()
	e.applyFog()
	e.updateOutcome(action)
	e.session.ReferenceAction = referenceAction(e.session.Turn)
	return e.State(), nil
}

func (e *Engine) resolveUser(unit *model.Unit, action model.Action) {
	switch action.Type {
	case "move":
		unit.Position = moveToward(unit.Position, *action.Target, 16)
	case "hold":
		unit.HP = min(unit.MaxHP, unit.HP+5)
	case "contest":
		target := e.nearestEnemy(*unit)
		if target != nil && distance(unit.Position, target.Position) <= 24 && unit.Cooldown == 0 {
			damage := 16 + e.rng.Intn(9)
			target.HP = max(0, target.HP-damage)
			target.Alive = target.HP > 0
			unit.Cooldown = 2
		}
	case "retreat":
		unit.Position = moveToward(unit.Position, model.Point{X: 8, Y: 50}, 18)
		unit.HP = min(unit.MaxHP, unit.HP+3)
	}
	e.session.Log = append(e.session.Log, model.LogEntry{
		Turn: e.session.Turn, Actor: "user", Action: action.Type,
		Message: fmt.Sprintf("You committed to %s.", action.Type),
	})
}

func (e *Engine) resolvePolicy(controlled *model.Unit) {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.Team == controlled.Team {
			continue
		}
		if distance(unit.Position, controlled.Position) <= 22 && unit.Cooldown == 0 {
			damage := 8 + e.rng.Intn(8)
			controlled.HP = max(0, controlled.HP-damage)
			controlled.Alive = controlled.HP > 0
			unit.Cooldown = 2
		} else {
			unit.Position = moveToward(unit.Position, controlled.Position, 7)
		}
	}
	e.session.Log = append(e.session.Log, model.LogEntry{
		Turn: e.session.Turn, Actor: "policy", Action: "respond",
		Message: "The deterministic opponent policy responded.",
	})
}

func (e *Engine) updateOutcome(action model.Action) {
	controlled := e.unit(e.moment.ControlledUnitID)
	enemiesAlive := 0
	for _, unit := range e.session.Units {
		if unit.Team != controlled.Team && unit.Alive {
			enemiesAlive++
		}
	}
	reward := map[string]int{"move": 4, "hold": -2, "contest": 12, "retreat": 3}[action.Type]
	if action.Type == referenceAction(e.session.Turn-1).Type {
		reward += 8
	}
	e.session.Score += reward
	e.session.WinProbability = clamp(e.session.WinProbability+float64(reward)/150, 0.01, 0.99)
	if !controlled.Alive {
		e.session.Status = "lost"
		e.session.WinProbability = 0.02
	} else if enemiesAlive == 0 {
		e.session.Status = "won"
		e.session.Score += 50
		e.session.WinProbability = 0.98
	} else if e.session.Turn >= e.session.MaxTurns {
		if e.session.Score >= 20 {
			e.session.Status = "won"
		} else {
			e.session.Status = "lost"
		}
	}
}

func (e *Engine) applyFog() {
	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil {
		return
	}
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		unit.Visible = unit.Team == controlled.Team || distance(unit.Position, controlled.Position) <= 34
	}
}

func (e *Engine) tickCooldowns() {
	for i := range e.session.Units {
		if e.session.Units[i].Cooldown > 0 {
			e.session.Units[i].Cooldown--
		}
	}
}

func (e *Engine) unit(id string) *model.Unit {
	for i := range e.session.Units {
		if e.session.Units[i].ID == id {
			return &e.session.Units[i]
		}
	}
	return nil
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

func referenceAction(turn int) model.Action {
	sequence := []string{"move", "contest", "retreat", "contest", "move"}
	action := model.Action{Type: sequence[turn%len(sequence)]}
	if action.Type == "move" {
		action.Target = &model.Point{X: 62, Y: 48}
	}
	return action
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

func cloneMoment(moment model.Moment) model.Moment {
	moment.Units = cloneUnits(moment.Units)
	moment.ReasonTags = slices.Clone(moment.ReasonTags)
	return moment
}

func cloneUnits(units []model.Unit) []model.Unit {
	return slices.Clone(units)
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
