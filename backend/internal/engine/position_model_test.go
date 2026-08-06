package engine

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

type stubModel struct {
	suggestions []PositionSuggestion
	err         error
	snapshot    ModelSnapshot
	calls       int
}

func (stub *stubModel) NextPositions(_ context.Context, snapshot ModelSnapshot) (ModelResult, error) {
	stub.snapshot = snapshot
	stub.calls++
	return ModelResult{
		ModelName: "test-policy", ModelVersion: "2026-08-05", Positions: stub.suggestions,
	}, stub.err
}

func momentWithTeammate() model.Moment {
	moment := testMoment()
	moment.Units[0].Position = model.Point{X: 10, Y: 50}
	moment.Units[1] = model.Unit{
		ID: "red-one", Team: "red", Role: "tank", Class: model.ClassTank,
		Position: model.Point{X: 70, Y: 50}, HP: 120, MaxHP: 160, Alive: true,
	}
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport,
		Position: model.Point{X: 20, Y: 50}, HP: 100, MaxHP: 110, Alive: true,
	})
	return moment
}

func TestPositionModelMovesTeammateAndOpponentWithinClassLimits(t *testing.T) {
	moment := momentWithTeammate()
	stub := &stubModel{suggestions: []PositionSuggestion{
		{UnitID: "blue-support", Position: model.Point{X: 100, Y: 50}},
		{UnitID: "red-one", Position: model.Point{X: 0, Y: 50}},
	}}
	engine := NewWithPositionModel(moment, "a", stub)
	state, err := engine.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}

	controlled := sessionUnit(t, state, "blue-carry")
	if controlled.Position != (model.Point{X: 10, Y: 50}) {
		t.Fatalf("position model moved the user-controlled unit: %+v", controlled.Position)
	}
	if controlled.HP != 75 {
		t.Fatalf("modeled teammate entered opponent combat logic: controlled HP=%d", controlled.HP)
	}
	if got := sessionUnit(t, state, "blue-support").Position; got != (model.Point{X: 28, Y: 50}) {
		t.Fatalf("support target was not clamped to 8 units: %+v", got)
	}
	if got := sessionUnit(t, state, "red-one").Position; got != (model.Point{X: 63, Y: 50}) {
		t.Fatalf("tank target was not clamped to 7 units: %+v", got)
	}
	if stub.calls != 1 || stub.snapshot.SchemaVersion != "1.1" ||
		stub.snapshot.StateScope != "authoritative_server_state" || stub.snapshot.Turn != 1 ||
		len(stub.snapshot.Units) != 3 || stub.snapshot.Units[0].HP != 75 {
		t.Fatalf("unexpected post-action model snapshot: calls=%d snapshot=%+v", stub.calls, stub.snapshot)
	}
	if !logContains(state, "eligible non-player units") {
		t.Fatal("expected generic position-model policy log")
	}

	records := engine.RolloutRecords()
	if len(records) != 1 || records[0].SchemaVersion != "1.1" ||
		records[0].SessionID != "a" || records[0].MomentID != "m1" || records[0].Turn != 1 ||
		records[0].ModelName != "test-policy" || records[0].ModelVersion != "2026-08-05" ||
		len(records[0].AcceptedPositions) != 2 {
		t.Fatalf("accepted mixed-team model response was not recorded: %+v", records)
	}
	records[0].AcceptedPositions[0].Position.X = 0
	if engine.RolloutRecords()[0].AcceptedPositions[0].Position.X != 100 {
		t.Fatal("rollout records were not returned defensively")
	}
	engine.Reset("a")
	if len(engine.RolloutRecords()) != 0 {
		t.Fatal("reset retained rollout records from the previous run")
	}
}

func TestPositionModelOmissionsUseTeamSpecificFallback(t *testing.T) {
	moment := momentWithTeammate()
	stub := &stubModel{suggestions: []PositionSuggestion{{
		UnitID: "blue-support", Position: model.Point{X: 100, Y: 50},
	}}}
	state, err := NewWithPositionModel(moment, "a", stub).Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionUnit(t, state, "blue-support").Position; got != (model.Point{X: 28, Y: 50}) {
		t.Fatalf("suggested teammate did not move: %+v", got)
	}
	if got := sessionUnit(t, state, "red-one").Position; got != (model.Point{X: 63, Y: 50}) {
		t.Fatalf("omitted opponent did not use deterministic chase: %+v", got)
	}

	emptyState, err := NewWithPositionModel(moment, "a", &stubModel{}).Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionUnit(t, emptyState, "blue-support").Position; got != (model.Point{X: 20, Y: 50}) {
		t.Fatalf("omitted teammate should hold position, got %+v", got)
	}
}

func TestInvalidModelSuggestionIsAtomic(t *testing.T) {
	moment := momentWithTeammate()
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-dead", Team: "blue", Role: "mage", Class: model.ClassMage,
		Position: model.Point{X: 25, Y: 50}, HP: 0, MaxHP: 95, Alive: false,
	})
	validAlly := PositionSuggestion{UnitID: "blue-support", Position: model.Point{X: 100, Y: 50}}
	tests := map[string][]PositionSuggestion{
		"controlled unit": {validAlly, {UnitID: "blue-carry", Position: model.Point{X: 11, Y: 50}}},
		"dead teammate":   {validAlly, {UnitID: "blue-dead", Position: model.Point{X: 26, Y: 50}}},
		"unknown unit":    {validAlly, {UnitID: "missing", Position: model.Point{X: 1, Y: 1}}},
		"duplicate unit":  {validAlly, {UnitID: "blue-support", Position: model.Point{X: 30, Y: 50}}},
		"non-finite":      {validAlly, {UnitID: "red-one", Position: model.Point{X: math.NaN(), Y: 50}}},
		"outside map":     {validAlly, {UnitID: "red-one", Position: model.Point{X: 101, Y: 50}}},
	}
	for name, suggestions := range tests {
		t.Run(name, func(t *testing.T) {
			baseline, baselineErr := New(moment, "a").Apply(model.Action{Type: "hold"})
			engine := NewWithPositionModel(moment, "a", &stubModel{suggestions: suggestions})
			modeled, modeledErr := engine.Apply(model.Action{Type: "hold"})
			if baselineErr != nil || modeledErr != nil {
				t.Fatalf("unexpected turn errors: %v, %v", baselineErr, modeledErr)
			}
			if !reflect.DeepEqual(baseline.Units, modeled.Units) ||
				baseline.Score != modeled.Score || baseline.WinProbability != modeled.WinProbability {
				t.Fatal("invalid mixed response partially mutated gameplay instead of deterministic fallback")
			}
			if !logContains(modeled, "response was unusable") || len(engine.RolloutRecords()) != 0 {
				t.Fatalf("invalid response was not rejected atomically: log=%+v records=%+v", modeled.Log, engine.RolloutRecords())
			}
		})
	}
}

func TestFailedOrUnidentifiedModelUsesDeterministicFallback(t *testing.T) {
	tests := map[string]PositionModel{
		"connector error": &stubModel{err: errors.New("model unavailable")},
		"missing identity": &modelWithoutIdentity{positions: []PositionSuggestion{{
			UnitID: "red-one", Position: model.Point{X: 60, Y: 50},
		}}},
	}
	for name, positionModel := range tests {
		t.Run(name, func(t *testing.T) {
			baseline, baselineErr := New(testMoment(), "a").Apply(model.Action{Type: "hold"})
			engine := NewWithPositionModel(testMoment(), "a", positionModel)
			modeled, modeledErr := engine.Apply(model.Action{Type: "hold"})
			if baselineErr != nil || modeledErr != nil {
				t.Fatalf("unexpected turn errors: %v, %v", baselineErr, modeledErr)
			}
			if !reflect.DeepEqual(baseline.Units, modeled.Units) || len(engine.RolloutRecords()) != 0 {
				t.Fatal("failed or unidentified model output did not fail closed")
			}
		})
	}
}

type modelWithoutIdentity struct {
	positions []PositionSuggestion
}

func (stub *modelWithoutIdentity) NextPositions(_ context.Context, _ ModelSnapshot) (ModelResult, error) {
	return ModelResult{Positions: stub.positions}, nil
}

func TestNoModelKeepsTeammateMovementDeterministic(t *testing.T) {
	moment := momentWithTeammate()
	a, errA := New(moment, "a").Apply(model.Action{Type: "hold"})
	b, errB := New(moment, "b").Apply(model.Action{Type: "hold"})
	a.ID, b.ID = "", ""
	if errA != nil || errB != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("no-model teammate trajectory was not deterministic: %v %v", errA, errB)
	}
	if got := sessionUnit(t, a, "blue-support").Position; got != (model.Point{X: 20, Y: 50}) {
		t.Fatalf("built-in policy unexpectedly moved teammate: %+v", got)
	}
}
