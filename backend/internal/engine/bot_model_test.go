package engine

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

type stubBotModel struct {
	actions      []BotActionSuggestion
	err          error
	modelName    string
	modelVersion string
	snapshot     BotSnapshot
	calls        int
}

func (stub *stubBotModel) NextActions(_ context.Context, snapshot BotSnapshot) (BotModelResult, error) {
	stub.snapshot = snapshot
	stub.calls++
	return BotModelResult{
		ModelName: stub.modelName, ModelVersion: stub.modelVersion, Actions: stub.actions,
	}, stub.err
}

func completeStub(actions []BotActionSuggestion) *stubBotModel {
	return &stubBotModel{
		actions: actions, modelName: "test-policy", modelVersion: "2026-08-05",
	}
}

func momentWithTeammate() model.Moment {
	moment := testMoment()
	moment.Units[0].Position = model.Point{X: 10, Y: 50}
	moment.Units[1] = model.Unit{
		ID: "red-one", Team: "red", Role: "tank", Class: model.ClassTank,
		Position: model.Point{X: 54, Y: 50}, HP: 120, MaxHP: 160, Alive: true,
	}
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport,
		Position: model.Point{X: 20, Y: 50}, HP: 100, MaxHP: 110, Alive: true,
	})
	return moment
}

func TestBotModelSuppliesEveryNonPlayerAction(t *testing.T) {
	moment := momentWithTeammate()
	allyTarget := model.Point{X: 100, Y: 50}
	enemyTarget := model.Point{X: 0, Y: 50}
	stub := completeStub([]BotActionSuggestion{
		{UnitID: "blue-support", Action: model.Action{Type: "move", Target: &allyTarget}},
		{UnitID: "red-one", Action: model.Action{Type: "move", Target: &enemyTarget}},
	})
	e := NewWithBotModel(moment, "a", stub)
	if initial := e.State().BotControl; initial.Source != "pending" {
		t.Fatalf("configured model should be pending before its first turn: %+v", initial)
	}
	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}

	if controlled := sessionUnit(t, state, "blue-carry"); controlled.Position != (model.Point{X: 10, Y: 50}) {
		t.Fatalf("bot model moved the user-controlled unit: %+v", controlled.Position)
	}
	if got := sessionUnit(t, state, "blue-support").Position; got != (model.Point{X: 28, Y: 50}) {
		t.Fatalf("support move was not clamped to its eight-unit class limit: %+v", got)
	}
	if got := sessionUnit(t, e.session, "red-one").Position; got != (model.Point{X: 47, Y: 50}) {
		t.Fatalf("tank move was not clamped to its seven-unit class limit: %+v", got)
	}
	if state.BotControl.Source != "external-model" || state.BotControl.ModelName != "test-policy" ||
		state.BotControl.ModelVersion != "2026-08-05" {
		t.Fatalf("session did not expose external model provenance: %+v", state.BotControl)
	}
	if stub.calls != 1 || stub.snapshot.SchemaVersion != BotModelSchemaVersion ||
		stub.snapshot.StateScope != "authoritative_server_state" || stub.snapshot.Turn != 1 ||
		stub.snapshot.ControlledUnitID != "blue-carry" || len(stub.snapshot.Units) != 3 ||
		len(stub.snapshot.LegalActions) != 4 || len(stub.snapshot.Projectiles) != 0 {
		t.Fatalf("unexpected bot-model snapshot: calls=%d snapshot=%+v", stub.calls, stub.snapshot)
	}
	if stub.snapshot.Projectiles == nil {
		t.Fatal("empty projectiles must remain an array in the bot-model request")
	}
	if !reflect.DeepEqual(stub.snapshot.LegalActions, []string{"move", "hold", "contest", "retreat"}) {
		t.Fatalf("snapshot exposed the wrong action vocabulary: %v", stub.snapshot.LegalActions)
	}
	if !logContains(state, "external model supplied every live bot action") {
		t.Fatalf("accepted model policy was not logged: %+v", state.Log)
	}

	records := e.RolloutRecords()
	if len(records) != 1 || records[0].SchemaVersion != BotModelSchemaVersion ||
		records[0].SessionID != "a" || records[0].MomentID != "m1" || records[0].Turn != 1 ||
		records[0].ModelName != "test-policy" || records[0].ModelVersion != "2026-08-05" ||
		len(records[0].AcceptedActions) != 2 {
		t.Fatalf("accepted bot response was not recorded: %+v", records)
	}
	records[0].AcceptedActions[0].Action.Target.X = 0
	if e.RolloutRecords()[0].AcceptedActions[0].Action.Target.X != 100 {
		t.Fatal("bot rollout records were not returned defensively")
	}
	e.Reset("a")
	if len(e.RolloutRecords()) != 0 {
		t.Fatal("reset retained bot rollout records from the previous run")
	}
}

func TestInvalidBotActionResponseFallsBackAtomically(t *testing.T) {
	moment := momentWithTeammate()
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-dead", Team: "blue", Role: "mage", Class: model.ClassMage,
		Position: model.Point{X: 25, Y: 50}, HP: 0, MaxHP: 95, Alive: false,
	})
	allyTarget := model.Point{X: 100, Y: 50}
	validAlly := BotActionSuggestion{UnitID: "blue-support", Action: model.Action{Type: "move", Target: &allyTarget}}
	point := func(x, y float64) *model.Point { return &model.Point{X: x, Y: y} }
	tests := map[string][]BotActionSuggestion{
		"controlled unit": {
			validAlly,
			{UnitID: "blue-carry", Action: model.Action{Type: "hold"}},
		},
		"dead unit": {
			validAlly,
			{UnitID: "blue-dead", Action: model.Action{Type: "hold"}},
		},
		"unknown unit": {
			validAlly,
			{UnitID: "missing", Action: model.Action{Type: "hold"}},
		},
		"duplicate unit": {
			validAlly,
			{UnitID: "blue-support", Action: model.Action{Type: "hold"}},
		},
		"non-finite move": {
			validAlly,
			{UnitID: "red-one", Action: model.Action{Type: "move", Target: point(math.NaN(), 50)}},
		},
		"outside map": {
			validAlly,
			{UnitID: "red-one", Action: model.Action{Type: "move", Target: point(101, 50)}},
		},
		"unknown action": {
			validAlly,
			{UnitID: "red-one", Action: model.Action{Type: "outplay"}},
		},
		"move without target": {
			validAlly,
			{UnitID: "red-one", Action: model.Action{Type: "move"}},
		},
		"non-move with target": {
			validAlly,
			{UnitID: "red-one", Action: model.Action{Type: "hold", Target: point(50, 50)}},
		},
		"missing eligible unit": {validAlly},
	}
	for name, actions := range tests {
		t.Run(name, func(t *testing.T) {
			baseline, baselineErr := New(moment, "a").Apply(model.Action{Type: "hold"})
			e := NewWithBotModel(moment, "a", completeStub(actions))
			modeled, modeledErr := e.Apply(model.Action{Type: "hold"})
			if baselineErr != nil || modeledErr != nil {
				t.Fatalf("unexpected turn errors: %v, %v", baselineErr, modeledErr)
			}
			if !reflect.DeepEqual(baseline.Units, modeled.Units) ||
				!reflect.DeepEqual(baseline.Projectiles, modeled.Projectiles) ||
				baseline.Advantage != modeled.Advantage || baseline.Status != modeled.Status {
				t.Fatal("invalid model response partially mutated gameplay instead of using fallback")
			}
			if modeled.BotControl.Source != "fallback" ||
				!logContains(modeled, "deterministic bot actions were applied") || len(e.RolloutRecords()) != 0 {
				t.Fatalf("invalid response was not rejected atomically: control=%+v log=%+v records=%+v",
					modeled.BotControl, modeled.Log, e.RolloutRecords())
			}
		})
	}
}

func TestFailedOrUnidentifiedBotModelUsesDeterministicFallback(t *testing.T) {
	redHold := BotActionSuggestion{UnitID: "red-one", Action: model.Action{Type: "hold"}}
	tests := map[string]BotModel{
		"connector error": &stubBotModel{
			err: errors.New("model unavailable"), modelName: "test-policy", modelVersion: "1",
		},
		"missing identity": &stubBotModel{actions: []BotActionSuggestion{redHold}},
	}
	for name, botModel := range tests {
		t.Run(name, func(t *testing.T) {
			baseline, baselineErr := New(testMoment(), "a").Apply(model.Action{Type: "hold"})
			e := NewWithBotModel(testMoment(), "a", botModel)
			modeled, modeledErr := e.Apply(model.Action{Type: "hold"})
			if baselineErr != nil || modeledErr != nil {
				t.Fatalf("unexpected turn errors: %v, %v", baselineErr, modeledErr)
			}
			if !reflect.DeepEqual(baseline.Units, modeled.Units) ||
				!reflect.DeepEqual(baseline.Projectiles, modeled.Projectiles) || len(e.RolloutRecords()) != 0 {
				t.Fatal("failed or unidentified bot-model output did not fail closed")
			}
			if modeled.BotControl.Source != "fallback" {
				t.Fatalf("fallback provenance was not exposed: %+v", modeled.BotControl)
			}
		})
	}
}

func TestNoModelKeepsBotMovementDeterministic(t *testing.T) {
	moment := momentWithTeammate()
	a, errA := New(moment, "a").Apply(model.Action{Type: "hold"})
	b, errB := New(moment, "b").Apply(model.Action{Type: "hold"})
	a.ID, b.ID = "", ""
	if errA != nil || errB != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("no-model bot actions were not deterministic: %v %v", errA, errB)
	}
	if got := sessionUnit(t, a, "blue-support").Position; got != (model.Point{X: 20, Y: 50}) {
		t.Fatalf("built-in support policy unexpectedly moved: %+v", got)
	}
	if a.BotControl.Source != "fallback" {
		t.Fatalf("no-model session omitted deterministic provenance: %+v", a.BotControl)
	}
}
