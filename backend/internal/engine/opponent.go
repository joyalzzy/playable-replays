package engine

import (
	"context"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	MapMin           = 0.0
	MapMax           = 100.0
	MaxSnapshotUnits = 64
)

type MapBounds struct {
	MinX float64 `json:"minX"`
	MaxX float64 `json:"maxX"`
	MinY float64 `json:"minY"`
	MaxY float64 `json:"maxY"`
}

type OpponentSnapshotUnit struct {
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

// OpponentSnapshot is privileged authoritative server state sent only to the
// operator-configured model endpoint. It is never constructed by the browser.
type OpponentSnapshot struct {
	SchemaVersion    string                 `json:"schemaVersion"`
	StateScope       string                 `json:"stateScope"`
	SessionID        string                 `json:"sessionId"`
	MomentID         string                 `json:"momentId"`
	Turn             int                    `json:"turn"`
	MapBounds        MapBounds              `json:"mapBounds"`
	ControlledUnitID string                 `json:"controlledUnitId"`
	Units            []OpponentSnapshotUnit `json:"units"`
}

type PositionSuggestion struct {
	UnitID   string      `json:"unitId"`
	Position model.Point `json:"position"`
}

type OpponentPositionModel interface {
	NextPositions(context.Context, OpponentSnapshot) ([]PositionSuggestion, error)
}
