package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

var ErrIllegalAction = errors.New("illegal action")

type Engine struct {
	moment        model.Moment
	session       model.Session
	rng           *rand.Rand
	opponentModel OpponentPositionModel
	rollouts      []OpponentRolloutRecord
}

type turnEffects struct {
	dodgeActive      bool
	dodgeEvaded      bool
	outplaySucceeded bool
}

func New(moment model.Moment, sessionID string) *Engine {
	return NewWithOpponentModel(moment, sessionID, nil)
}

func NewWithOpponentModel(moment model.Moment, sessionID string, opponentModel OpponentPositionModel) *Engine {
	e := &Engine{moment: cloneMoment(moment), opponentModel: opponentModel}
	for i := range e.moment.Units {
		e.moment.Units[i] = model.ApplyClassProfile(e.moment.Units[i])
	}
	e.Reset(sessionID)
	return e
}

func (e *Engine) Reset(sessionID string) model.Session {
	e.rng = rand.New(rand.NewSource(e.moment.Seed))
	e.rollouts = nil
	e.session = model.Session{
		ID:               sessionID,
		MomentID:         e.moment.ID,
		ControlledUnitID: e.moment.ControlledUnitID,
		MaxTurns:         e.moment.MaxTurns,
		Status:           "active",
		Score:            0,
		WinProbability:   clamp(0.5-e.moment.Signals.ResourceAsymmetry*0.1, 0.05, 0.95),
		LegalActions:     []string{"move", "hold", "contest", "retreat", "dodge", "outplay"},
		Units:            cloneUnits(e.moment.Units),
		Log:              []model.LogEntry{},
	}
	e.session.ReferenceAction = e.referenceAction(0)
	e.applyFog()
	return e.State()
}

// RolloutRecords returns a defensive copy of accepted external-model outputs.
// Records remain server-side and live only as long as this in-memory engine.
func (e *Engine) RolloutRecords() []OpponentRolloutRecord {
	records := make([]OpponentRolloutRecord, len(e.rollouts))
	for index, record := range e.rollouts {
		records[index] = record
		records[index].AcceptedPositions = slices.Clone(record.AcceptedPositions)
	}
	return records
}

func (e *Engine) State() model.Session {
	state := e.session
	state.Units = cloneUnits(e.session.Units)
	state.Log = slices.Clone(e.session.Log)
	state.LegalActions = slices.Clone(e.session.LegalActions)
	if e.session.ReferenceAction.Target != nil {
		target := *e.session.ReferenceAction.Target
		state.ReferenceAction.Target = &target
	}
	return state
}

func (e *Engine) Apply(action model.Action) (model.Session, error) {
	return e.ApplyContext(context.Background(), action)
}

func (e *Engine) ApplyContext(ctx context.Context, action model.Action) (model.Session, error) {
	if e.session.Status != "active" {
		return e.State(), fmt.Errorf("%w: session is complete", ErrIllegalAction)
	}
	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return e.State(), fmt.Errorf("%w: controlled unit is unavailable", ErrIllegalAction)
	}
	if err := e.validateAction(action, *controlled); err != nil {
		return e.State(), err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	e.session.Turn++
	effects := e.resolveUser(controlled, action)
	e.resolvePolicy(ctx, controlled, &effects)
	e.tickCooldowns()
	e.applyFog()
	e.updateOutcome(action, effects)
	e.session.ReferenceAction = e.referenceAction(e.session.Turn)
	return e.State(), nil
}

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

func (e *Engine) resolvePolicy(ctx context.Context, controlled *model.Unit, effects *turnEffects) {
	suggestions, modelUsed, fallback := e.modelSuggestions(ctx, *controlled)
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.ID == controlled.ID {
			continue
		}

		if desired, ok := suggestions[unit.ID]; ok {
			unit.Position = moveToward(unit.Position, desired, unit.MoveRange)
		} else if unit.Team != controlled.Team && distance(unit.Position, controlled.Position) > unit.AttackRange {
			unit.Position = moveToward(unit.Position, controlled.Position, unit.MoveRange)
		}
		if unit.Team == controlled.Team {
			continue
		}

		if distance(unit.Position, controlled.Position) <= unit.AttackRange && unit.Cooldown == 0 {
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

	message := "The deterministic opponent policy responded."
	action := "respond"
	if fallback {
		action = "fallback"
		message = "The position model response was unusable; teammates held position and the deterministic opponent policy responded."
	} else if modelUsed {
		action = "model-respond"
		message = "The position model responded for non-player units; authoritative class movement limits were applied."
	}
	e.session.Log = append(e.session.Log, model.LogEntry{
		Turn: e.session.Turn, Actor: "policy", Action: action, Message: message,
	})
}

func (e *Engine) modelSuggestions(ctx context.Context, controlled model.Unit) (map[string]model.Point, bool, bool) {
	if e.opponentModel == nil {
		return nil, false, false
	}
	snapshot := e.opponentSnapshot()
	result, err := e.opponentModel.NextPositions(ctx, snapshot)
	if err != nil {
		return nil, false, true
	}
	if strings.TrimSpace(result.ModelName) == "" || strings.TrimSpace(result.ModelVersion) == "" {
		return nil, false, true
	}
	eligible := make(map[string]struct{})
	for _, unit := range e.session.Units {
		if unit.Alive && unit.ID != controlled.ID {
			eligible[unit.ID] = struct{}{}
		}
	}
	validated := make(map[string]model.Point, len(result.Positions))
	accepted := make([]PositionSuggestion, 0, len(result.Positions))
	for _, proposal := range result.Positions {
		if _, ok := eligible[proposal.UnitID]; !ok {
			return nil, false, true
		}
		if _, duplicate := validated[proposal.UnitID]; duplicate {
			return nil, false, true
		}
		if !pointFinite(proposal.Position) || !pointInBounds(proposal.Position) {
			return nil, false, true
		}
		validated[proposal.UnitID] = proposal.Position
		accepted = append(accepted, proposal)
	}
	e.rollouts = append(e.rollouts, OpponentRolloutRecord{
		SessionID: snapshot.SessionID, MomentID: snapshot.MomentID, Turn: snapshot.Turn,
		ModelName: result.ModelName, ModelVersion: result.ModelVersion,
		AcceptedPositions: accepted,
	})
	return validated, true, false
}

func (e *Engine) opponentSnapshot() OpponentSnapshot {
	count := min(len(e.session.Units), MaxSnapshotUnits)
	units := make([]OpponentSnapshotUnit, 0, count)
	for _, unit := range e.session.Units[:count] {
		units = append(units, OpponentSnapshotUnit{
			ID: unit.ID, Team: unit.Team, Role: unit.Role, Class: unit.Class, Position: unit.Position,
			HP: unit.HP, MaxHP: unit.MaxHP, MoveRange: unit.MoveRange,
			AttackRange: unit.AttackRange, Cooldown: unit.Cooldown,
			Visible: unit.Visible, Alive: unit.Alive,
		})
	}
	return OpponentSnapshot{
		SchemaVersion: "1.0", StateScope: "authoritative_server_state",
		SessionID: e.session.ID, MomentID: e.session.MomentID, Turn: e.session.Turn,
		MapBounds:        MapBounds{MinX: MapMin, MaxX: MapMax, MinY: MapMin, MaxY: MapMax},
		ControlledUnitID: e.session.ControlledUnitID, Units: units,
	}
}

func (e *Engine) updateOutcome(action model.Action, effects turnEffects) {
	controlled := e.unit(e.moment.ControlledUnitID)
	enemiesAlive := 0
	for _, unit := range e.session.Units {
		if unit.Team != controlled.Team && unit.Alive {
			enemiesAlive++
		}
	}
	reward := map[string]int{"move": 4, "hold": -2, "contest": 12, "retreat": 3}[action.Type]
	successfulSpecial := false
	switch action.Type {
	case "dodge":
		if effects.dodgeEvaded {
			reward = 8
			successfulSpecial = true
		} else {
			reward = 2
		}
	case "outplay":
		if effects.outplaySucceeded {
			reward = 16
			successfulSpecial = true
		} else {
			reward = 0
		}
	}
	if action.Type == e.referenceAction(e.session.Turn-1).Type &&
		(action.Type != "dodge" && action.Type != "outplay" || successfulSpecial) {
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
