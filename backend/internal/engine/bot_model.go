package engine

import (
	"context"
	"slices"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	BotModelSchemaVersion = "2.0"
	MaxSnapshotUnits      = 64
)

type MapBounds struct {
	MinX float64 `json:"minX"`
	MaxX float64 `json:"maxX"`
	MinY float64 `json:"minY"`
	MaxY float64 `json:"maxY"`
}

type BotSnapshotUnit struct {
	ID          string          `json:"id"`
	Team        string          `json:"team"`
	Role        string          `json:"role"`
	Class       model.UnitClass `json:"class"`
	Policy      string          `json:"fallbackPolicy"`
	Position    model.Point     `json:"position"`
	HP          int             `json:"hp"`
	MaxHP       int             `json:"maxHp"`
	MoveRange   float64         `json:"moveRange"`
	AttackRange float64         `json:"attackRange"`
	Cooldown    int             `json:"cooldownTurns"`
	Shield      int             `json:"shield"`
	Guarded     bool            `json:"guarded"`
	Visible     bool            `json:"visible"`
	Alive       bool            `json:"alive"`
}

// BotSnapshot is privileged authoritative server state sent only to the
// operator-configured model bridge. It is never constructed by the browser.
type BotSnapshot struct {
	SchemaVersion    string                `json:"schemaVersion"`
	StateScope       string                `json:"stateScope"`
	SessionID        string                `json:"sessionId"`
	MomentID         string                `json:"momentId"`
	Turn             int                   `json:"turn"`
	MapBounds        MapBounds             `json:"mapBounds"`
	ControlledUnitID string                `json:"controlledUnitId"`
	LegalActions     []string              `json:"legalActions"`
	Objective        *model.ObjectiveState `json:"objective,omitempty"`
	Projectiles      []model.Projectile    `json:"projectiles"`
	Units            []BotSnapshotUnit     `json:"units"`
}

type BotActionSuggestion struct {
	UnitID string       `json:"unitId"`
	Action model.Action `json:"action"`
}

type BotModelResult struct {
	ModelName    string
	ModelVersion string
	Actions      []BotActionSuggestion
}

type BotRolloutRecord struct {
	SchemaVersion   string                `json:"schemaVersion"`
	SessionID       string                `json:"sessionId"`
	MomentID        string                `json:"momentId"`
	Turn            int                   `json:"turn"`
	ModelName       string                `json:"modelName"`
	ModelVersion    string                `json:"modelVersion"`
	AcceptedActions []BotActionSuggestion `json:"acceptedActions"`
}

type BotModel interface {
	NextActions(context.Context, BotSnapshot) (BotModelResult, error)
}

func (e *Engine) RolloutRecords() []BotRolloutRecord {
	records := make([]BotRolloutRecord, len(e.rollouts))
	for index, record := range e.rollouts {
		records[index] = record
		records[index].AcceptedActions = make([]BotActionSuggestion, len(record.AcceptedActions))
		for actionIndex, suggestion := range record.AcceptedActions {
			records[index].AcceptedActions[actionIndex] = suggestion
			records[index].AcceptedActions[actionIndex].Action = cloneAction(suggestion.Action)
		}
	}
	return records
}

func (e *Engine) modelActions(ctx context.Context) (map[string]model.Action, bool, bool) {
	if e.botModel == nil {
		return nil, false, false
	}
	snapshot := e.botSnapshot()
	result, err := e.botModel.NextActions(ctx, snapshot)
	if err != nil || len(result.Actions) > MaxSnapshotUnits {
		return nil, false, true
	}
	if strings.TrimSpace(result.ModelName) == "" || strings.TrimSpace(result.ModelVersion) == "" {
		return nil, false, true
	}

	eligible := make(map[string]struct{}, len(snapshot.Units))
	for _, unit := range snapshot.Units {
		if unit.Alive && unit.ID != snapshot.ControlledUnitID {
			eligible[unit.ID] = struct{}{}
		}
	}
	if len(result.Actions) != len(eligible) {
		return nil, false, true
	}

	validated := make(map[string]model.Action, len(result.Actions))
	accepted := make([]BotActionSuggestion, 0, len(result.Actions))
	for _, proposal := range result.Actions {
		if _, ok := eligible[proposal.UnitID]; !ok {
			return nil, false, true
		}
		if _, duplicate := validated[proposal.UnitID]; duplicate {
			return nil, false, true
		}
		if !slices.Contains(actionTypes, proposal.Action.Type) {
			return nil, false, true
		}
		if proposal.Action.Type == "move" {
			if proposal.Action.Target == nil || !pointFinite(*proposal.Action.Target) || !pointInBounds(*proposal.Action.Target) {
				return nil, false, true
			}
		} else if proposal.Action.Target != nil {
			return nil, false, true
		}
		validated[proposal.UnitID] = cloneAction(proposal.Action)
		accepted = append(accepted, BotActionSuggestion{UnitID: proposal.UnitID, Action: cloneAction(proposal.Action)})
	}

	e.rollouts = append(e.rollouts, BotRolloutRecord{
		SchemaVersion: snapshot.SchemaVersion,
		SessionID:     snapshot.SessionID, MomentID: snapshot.MomentID, Turn: snapshot.Turn,
		ModelName: result.ModelName, ModelVersion: result.ModelVersion,
		AcceptedActions: accepted,
	})
	return validated, true, false
}

func (e *Engine) botSnapshot() BotSnapshot {
	count := min(len(e.session.Units), MaxSnapshotUnits)
	units := make([]BotSnapshotUnit, 0, count)
	for _, unit := range e.session.Units[:count] {
		units = append(units, BotSnapshotUnit{
			ID: unit.ID, Team: unit.Team, Role: unit.Role, Class: unit.Class, Policy: unit.Policy,
			Position: unit.Position, HP: unit.HP, MaxHP: unit.MaxHP,
			MoveRange: unit.MoveRange, AttackRange: unit.AttackRange, Cooldown: unit.Cooldown,
			Shield: unit.Shield, Guarded: unit.Guarded,
			Visible: unit.Visible, Alive: unit.Alive,
		})
	}
	var objective *model.ObjectiveState
	if e.session.Objective != nil {
		cloned := *e.session.Objective
		objective = &cloned
	}
	return BotSnapshot{
		SchemaVersion: BotModelSchemaVersion, StateScope: "authoritative_server_state",
		SessionID: e.session.ID, MomentID: e.session.MomentID, Turn: e.session.Turn,
		MapBounds:        MapBounds{MinX: MapMin, MaxX: MapMax, MinY: MapMin, MaxY: MapMax},
		ControlledUnitID: e.session.ControlledUnitID,
		LegalActions:     slices.Clone(actionTypes), Objective: objective,
		Projectiles: append([]model.Projectile{}, e.session.Projectiles...), Units: units,
	}
}
