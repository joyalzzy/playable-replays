package engine

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

var ErrIllegalAction = errors.New("illegal action")

type Engine struct {
	moment        model.Moment
	session       model.Session
	rng           *rand.Rand
	positionModel PositionModel
	rollouts      []ModelRolloutRecord
}

type turnEffects struct {
	dodgeActive      bool
	dodgeEvaded      bool
	outplaySucceeded bool
}

func New(moment model.Moment, sessionID string) *Engine {
	return NewWithPositionModel(moment, sessionID, nil)
}

func NewWithPositionModel(moment model.Moment, sessionID string, positionModel PositionModel) *Engine {
	e := &Engine{moment: cloneMoment(moment), positionModel: positionModel}
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
