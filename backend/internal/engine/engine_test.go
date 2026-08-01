package engine

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

type stubOpponentModel struct {
	suggestions []PositionSuggestion
	err         error
	snapshot    OpponentSnapshot
}

func (stub *stubOpponentModel) NextPositions(_ context.Context, snapshot OpponentSnapshot) ([]PositionSuggestion, error) {
	stub.snapshot = snapshot
	return stub.suggestions, stub.err
}

func testMoment() model.Moment {
	return model.Moment{
		ID: "m1", Slug: "test", Seed: 7, MaxTurns: 3, ControlledUnitID: "blue-carry",
		Units: []model.Unit{
			{ID: "blue-carry", Team: "blue", Role: "carry", Class: model.ClassMarksman, Position: model.Point{X: 30, Y: 50}, HP: 70, MaxHP: 90, Alive: true},
			{ID: "red-one", Team: "red", Role: "fighter", Class: model.ClassFighter, Position: model.Point{X: 48, Y: 50}, HP: 80, MaxHP: 125, Alive: true},
		},
	}
}

func sessionUnit(t *testing.T, state model.Session, id string) model.Unit {
	t.Helper()
	for _, unit := range state.Units {
		if unit.ID == id {
			return unit
		}
	}
	t.Fatalf("unit %q was not found", id)
	return model.Unit{}
}

func logContains(state model.Session, fragment string) bool {
	for _, entry := range state.Log {
		if strings.Contains(strings.ToLower(entry.Message), strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func TestDeterministicSequence(t *testing.T) {
	a := New(testMoment(), "a")
	b := New(testMoment(), "b")
	actions := []model.Action{{Type: "contest"}, {Type: "retreat"}, {Type: "hold"}}
	for _, action := range actions {
		stateA, errA := a.Apply(action)
		stateB, errB := b.Apply(action)
		stateA.ID, stateB.ID = "", ""
		if errA != nil || errB != nil || !reflect.DeepEqual(stateA, stateB) {
			t.Fatalf("expected identical state: %v %v", errA, errB)
		}
	}
}

func TestMoveRequiresTarget(t *testing.T) {
	e := New(testMoment(), "a")
	before := e.State()
	after, err := e.Apply(model.Action{Type: "move"})
	if err == nil {
		t.Fatal("expected illegal action")
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("illegal action mutated state")
	}
}

func TestMoveRejectsTargetBeyondClassLimitWithoutMutation(t *testing.T) {
	e := New(testMoment(), "a")
	before := e.State()
	after, err := e.Apply(model.Action{Type: "move", Target: &model.Point{X: 42, Y: 50}})
	if !errors.Is(err, ErrIllegalAction) {
		t.Fatalf("expected illegal action, got %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("over-range movement mutated state")
	}
}

func TestClassMovementLimitsTankAndAssassin(t *testing.T) {
	tests := []struct {
		class model.UnitClass
		limit float64
	}{
		{model.ClassTank, 7},
		{model.ClassAssassin, 13},
	}
	for _, test := range tests {
		t.Run(string(test.class), func(t *testing.T) {
			moment := testMoment()
			moment.Units[0] = model.Unit{
				ID: "blue-carry", Team: "blue", Role: string(test.class), Class: test.class,
				Position: model.Point{X: 30, Y: 50}, HP: 80, MaxHP: 100, Alive: true,
			}
			profile, _ := model.Profile(test.class)
			moment.Units[0].MaxHP = profile.MaxHP
			target := model.Point{X: 30 + test.limit, Y: 50}
			state, err := New(moment, "a").Apply(model.Action{Type: "move", Target: &target})
			if err != nil {
				t.Fatal(err)
			}
			position := sessionUnit(t, state, "blue-carry").Position
			if math.Abs(position.X-target.X) > 1e-9 || position.Y != target.Y {
				t.Fatalf("expected movement to %+v, got %+v", target, position)
			}
		})
	}
}

func TestOpponentModelTargetIsClampedByClassMovement(t *testing.T) {
	moment := testMoment()
	moment.Units[0].Position = model.Point{X: 10, Y: 50}
	moment.Units[1] = model.Unit{
		ID: "red-one", Team: "red", Role: "tank", Class: model.ClassTank,
		Position: model.Point{X: 50, Y: 50}, HP: 120, MaxHP: 160, Alive: true,
	}
	stub := &stubOpponentModel{suggestions: []PositionSuggestion{
		{UnitID: "red-one", Position: model.Point{X: 100, Y: 50}},
	}}
	state, err := NewWithOpponentModel(moment, "a", stub).Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	red := sessionUnit(t, state, "red-one")
	if red.Position != (model.Point{X: 57, Y: 50}) {
		t.Fatalf("tank target was not clamped to 7 units: %+v", red.Position)
	}
	if stub.snapshot.SchemaVersion != "1.0" || stub.snapshot.StateScope != "authoritative_server_state" || stub.snapshot.Turn != 1 {
		t.Fatalf("unexpected snapshot metadata: %+v", stub.snapshot)
	}
	if !logContains(state, "model responded") {
		t.Fatal("expected model-source policy log")
	}
}

func TestInvalidOrFailedModelUsesDeterministicFallback(t *testing.T) {
	tests := map[string]*stubOpponentModel{
		"unknown unit":    {suggestions: []PositionSuggestion{{UnitID: "blue-carry", Position: model.Point{X: 31, Y: 50}}}},
		"connector error": {err: errors.New("model unavailable")},
	}
	for name, stub := range tests {
		t.Run(name, func(t *testing.T) {
			baseline, baselineErr := New(testMoment(), "a").Apply(model.Action{Type: "hold"})
			modeled, modeledErr := NewWithOpponentModel(testMoment(), "a", stub).Apply(model.Action{Type: "hold"})
			if baselineErr != nil || modeledErr != nil {
				t.Fatalf("unexpected turn errors: %v, %v", baselineErr, modeledErr)
			}
			if !reflect.DeepEqual(baseline.Units, modeled.Units) || baseline.Score != modeled.Score || baseline.WinProbability != modeled.WinProbability {
				t.Fatal("fallback did not preserve deterministic gameplay")
			}
			if !logContains(modeled, "response was unusable") {
				t.Fatal("expected explicit fallback log")
			}
		})
	}
}

func TestDodgeLogsEvadedSkillshotAndRewardsActualEvasion(t *testing.T) {
	moment := testMoment()
	moment.Units[1].Position = model.Point{X: 38, Y: 50}
	state, err := New(moment, "a").Apply(model.Action{Type: "dodge"})
	if err != nil {
		t.Fatal(err)
	}
	if !logContains(state, "dodged red-one's skillshot") {
		t.Fatalf("expected dodge log, got %+v", state.Log)
	}
	if state.Score != 8 {
		t.Fatalf("expected successful dodge reward, got %d", state.Score)
	}
	if sessionUnit(t, state, "blue-carry").HP != 70 {
		t.Fatal("a successfully dodged skillshot dealt damage")
	}
}

func TestDodgeWithoutThreatGetsOnlyRepositionReward(t *testing.T) {
	moment := testMoment()
	moment.Units[1].Position = model.Point{X: 90, Y: 90}
	state, err := New(moment, "a").Apply(model.Action{Type: "dodge"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Score != 2 || logContains(state, "dodged red-one's skillshot") {
		t.Fatalf("non-evasion dodge was rewarded or logged as a success: score=%d log=%+v", state.Score, state.Log)
	}
}

func TestOutplayLogsSuccessAndFailureTruthfully(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		moment := testMoment()
		moment.Units[1].Position = model.Point{X: 38, Y: 50}
		state, err := New(moment, "a").Apply(model.Action{Type: "outplay"})
		if err != nil {
			t.Fatal(err)
		}
		if !logContains(state, "outplayed red-one") || state.Score != 16 {
			t.Fatalf("expected successful outplay, got score=%d log=%+v", state.Score, state.Log)
		}
		if sessionUnit(t, state, "red-one").HP >= 80 {
			t.Fatal("successful outplay did not damage the target")
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		moment := testMoment()
		moment.Units[1].Position = model.Point{X: 90, Y: 50}
		state, err := New(moment, "a").Apply(model.Action{Type: "outplay"})
		if err != nil {
			t.Fatal(err)
		}
		if !logContains(state, "outplay was unavailable") || state.Score != 0 {
			t.Fatalf("failed outplay was reported or rewarded incorrectly: score=%d log=%+v", state.Score, state.Log)
		}
	})
}

func TestFogHidesDistantEnemy(t *testing.T) {
	moment := testMoment()
	moment.Units[1].Position = model.Point{X: 99, Y: 99}
	state := New(moment, "a").State()
	if state.Units[1].Visible {
		t.Fatal("distant enemy should be hidden")
	}
}

func TestResetRestoresState(t *testing.T) {
	e := New(testMoment(), "a")
	_, _ = e.Apply(model.Action{Type: "contest"})
	reset := e.Reset("a")
	fresh := New(testMoment(), "a").State()
	if !reflect.DeepEqual(reset, fresh) {
		t.Fatal("reset did not restore deterministic state")
	}
}
