package engine

import (
	"context"
	"slices"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	PositionModelSchemaVersion = "1.1"
	MaxSnapshotUnits           = 64
)

type MapBounds struct {
	MinX float64 `json:"minX"`
	MaxX float64 `json:"maxX"`
	MinY float64 `json:"minY"`
	MaxY float64 `json:"maxY"`
}

type ModelSnapshotUnit struct {
	ID          string          `json:"id"`
	Team        string          `json:"team"`
	Role        string          `json:"role"`
	Class       model.UnitClass `json:"class"`
	Position    model.Point     `json:"position"`
	HP          int             `json:"hp"`
	MaxHP       int             `json:"maxHp"`
	MoveRange   float64         `json:"moveRange"`
	AttackRange float64         `json:"attackRange"`
	Cooldown    int             `json:"cooldownTurns"`
	Visible     bool            `json:"visible"`
	Alive       bool            `json:"alive"`
}

// ModelSnapshot is privileged authoritative server state sent only to the
// operator-configured model endpoint. It is never constructed by the browser.
type ModelSnapshot struct {
	SchemaVersion    string              `json:"schemaVersion"`
	StateScope       string              `json:"stateScope"`
	SessionID        string              `json:"sessionId"`
	MomentID         string              `json:"momentId"`
	Turn             int                 `json:"turn"`
	MapBounds        MapBounds           `json:"mapBounds"`
	ControlledUnitID string              `json:"controlledUnitId"`
	Units            []ModelSnapshotUnit `json:"units"`
}

type PositionSuggestion struct {
	UnitID   string      `json:"unitId"`
	Position model.Point `json:"position"`
}

type ModelResult struct {
	ModelName    string
	ModelVersion string
	Positions    []PositionSuggestion
}

// ModelRolloutRecord is privileged, session-scoped metadata retained for
// reproducibility. It is not part of the browser Session response.
type ModelRolloutRecord struct {
	SchemaVersion     string               `json:"schemaVersion"`
	SessionID         string               `json:"sessionId"`
	MomentID          string               `json:"momentId"`
	Turn              int                  `json:"turn"`
	ModelName         string               `json:"modelName"`
	ModelVersion      string               `json:"modelVersion"`
	AcceptedPositions []PositionSuggestion `json:"acceptedPositions"`
}

type PositionModel interface {
	NextPositions(context.Context, ModelSnapshot) (ModelResult, error)
}

// RolloutRecords returns a defensive copy of accepted external-model outputs.
// Records remain server-side and live only as long as this in-memory engine.
func (e *Engine) RolloutRecords() []ModelRolloutRecord {
	records := make([]ModelRolloutRecord, len(e.rollouts))
	for index, record := range e.rollouts {
		records[index] = record
		records[index].AcceptedPositions = slices.Clone(record.AcceptedPositions)
	}
	return records
}

func (e *Engine) modelSuggestions(ctx context.Context) (map[string]model.Point, bool, bool) {
	if e.positionModel == nil {
		return nil, false, false
	}
	snapshot := e.modelSnapshot()
	result, err := e.positionModel.NextPositions(ctx, snapshot)
	if err != nil || len(result.Positions) > MaxSnapshotUnits {
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
	e.rollouts = append(e.rollouts, ModelRolloutRecord{
		SchemaVersion: snapshot.SchemaVersion,
		SessionID:     snapshot.SessionID, MomentID: snapshot.MomentID, Turn: snapshot.Turn,
		ModelName: result.ModelName, ModelVersion: result.ModelVersion,
		AcceptedPositions: accepted,
	})
	return validated, true, false
}

func (e *Engine) modelSnapshot() ModelSnapshot {
	count := min(len(e.session.Units), MaxSnapshotUnits)
	units := make([]ModelSnapshotUnit, 0, count)
	for _, unit := range e.session.Units[:count] {
		units = append(units, ModelSnapshotUnit{
			ID: unit.ID, Team: unit.Team, Role: unit.Role, Class: unit.Class, Position: unit.Position,
			HP: unit.HP, MaxHP: unit.MaxHP, MoveRange: unit.MoveRange,
			AttackRange: unit.AttackRange, Cooldown: unit.Cooldown,
			Visible: unit.Visible, Alive: unit.Alive,
		})
	}
	return ModelSnapshot{
		SchemaVersion: PositionModelSchemaVersion, StateScope: "authoritative_server_state",
		SessionID: e.session.ID, MomentID: e.session.MomentID, Turn: e.session.Turn,
		MapBounds:        MapBounds{MinX: MapMin, MaxX: MapMax, MinY: MapMin, MaxY: MapMax},
		ControlledUnitID: e.session.ControlledUnitID, Units: units,
	}
}
