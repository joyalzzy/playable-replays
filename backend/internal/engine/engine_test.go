package engine

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func testMoment() model.Moment {
	return model.Moment{
		ID: "m1", Slug: "test", Seed: 7, MaxTurns: 3, ControlledUnitID: "blue-carry",
		Units: []model.Unit{
			{ID: "blue-carry", Team: "blue", Role: "carry", Class: model.ClassMarksman, Policy: "controlled", Position: model.Point{X: 30, Y: 50}, HP: 70, MaxHP: 90, AttackRange: 28, AttackDamage: 20, MoveRange: 11, MoveSpeed: 11, Armor: 12, VisionRange: 34, AttackCooldown: 2, Alive: true},
			{ID: "red-one", Team: "red", Role: "fighter", Class: model.ClassFighter, Policy: "aggressive", Position: model.Point{X: 48, Y: 50}, HP: 80, MaxHP: 125, AttackRange: 14, AttackDamage: 16, MoveRange: 10, MoveSpeed: 10, Armor: 15, VisionRange: 32, AttackCooldown: 2, Alive: true},
		},
		Rules: model.ScenarioRules{
			InitialAdvantage: .5,
			Victory: model.VictoryRules{
				Kind: "skirmish", Description: "Finish with the stronger tactical state.",
				DefeatDescription: "The controlled unit was eliminated.", SafeZone: model.Point{X: 8, Y: 50},
			},
			ReferencePlan: []model.Action{{Type: "contest"}, {Type: "hold"}, {Type: "hold"}},
			ActionDefaults: map[string]model.Action{
				"move":    {Type: "move", Target: &model.Point{X: 40, Y: 50}},
				"hold":    {Type: "hold"},
				"contest": {Type: "contest"},
				"retreat": {Type: "retreat"},
			},
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
	actions := []model.Action{{Type: "hold"}, {Type: "move", Target: &model.Point{X: 36, Y: 52}}, {Type: "hold"}}
	for _, action := range actions {
		stateA, errA := a.Apply(action)
		stateB, errB := b.Apply(action)
		stateA.ID, stateB.ID = "", ""
		if !reflect.DeepEqual(errA, errB) || !reflect.DeepEqual(stateA, stateB) {
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

func TestHiddenEnemyIsRemovedFromPublicState(t *testing.T) {
	moment := testMoment()
	moment.Units[1].Position = model.Point{X: 99, Y: 99}
	state := New(moment, "a").State()
	if len(state.Units) != 1 || state.Units[0].ID != "blue-carry" {
		t.Fatalf("hidden enemy leaked into public state: %+v", state.Units)
	}
	if !state.VisionLimited || state.UnknownEnemyCount != 1 || state.VisibleEnemyCount != 0 {
		t.Fatalf("expected explicit limited-vision counts: %+v", state)
	}
}

func TestMechanicBriefingIsIncludedInPublicSessionState(t *testing.T) {
	moment := testMoment()
	moment.MechanicBriefing = &model.MechanicBriefing{Mechanics: []model.ScenarioMechanic{{
		ElementID: "special-zone", Name: "Special Zone", Description: "Builds control.", RoleInScenario: "Creates the decision window.",
	}}}
	state := New(moment, "a").State()
	if state.MechanicBriefing == nil || len(state.MechanicBriefing.Mechanics) != 1 || state.MechanicBriefing.Mechanics[0].ElementID != "special-zone" {
		t.Fatalf("expected authored mechanic briefing in public state: %+v", state.MechanicBriefing)
	}
	state.MechanicBriefing.Mechanics[0].Name = "mutated"
	if New(moment, "b").State().MechanicBriefing.Mechanics[0].Name != "Special Zone" {
		t.Fatal("public mechanic briefing should be cloned")
	}
}

func TestContestProducesCausalDamageLog(t *testing.T) {
	state, err := New(testMoment(), "a").Apply(model.Action{Type: "contest"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range state.Log {
		if entry.Kind == "damage" && entry.Value > 0 && strings.Contains(entry.Message, "HP") {
			return
		}
	}
	t.Fatalf("expected a causal damage event, got %+v", state.Log)
}

func TestObjectiveHasExplicitVictoryCondition(t *testing.T) {
	moment := testMoment()
	moment.MaxTurns = 2
	moment.Units[0].Position = model.Point{X: 50, Y: 50}
	moment.Units[1].Position = model.Point{X: 95, Y: 95}
	moment.Rules.Objective = &model.ObjectiveRules{ID: "core", Label: "Core", Position: model.Point{X: 50, Y: 50}, Radius: 10, CaptureTurns: 1}
	moment.Rules.Victory.Kind = "secure-objective"
	moment.Rules.ReferencePlan = []model.Action{{Type: "hold"}, {Type: "hold"}}
	state, err := New(moment, "a").Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "won" || state.Objective == nil || state.Objective.Status != "secured-blue" || state.OutcomeReason == "" {
		t.Fatalf("expected authored objective victory, got %+v", state)
	}
	if len(state.ReferenceOutcomes) != len(actionTypes) {
		t.Fatalf("expected one reference rollout per first action, got %d", len(state.ReferenceOutcomes))
	}
}

func TestReferenceHiddenUntilFirstCommitAndRolloutsUntilEnd(t *testing.T) {
	e := New(testMoment(), "a")
	initial := e.State()
	if initial.LastReferenceAction != nil || len(initial.ReferenceOutcomes) != 0 || initial.BestCase != nil {
		t.Fatal("reference information should not lead the user's first decision")
	}
	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if state.LastReferenceAction == nil || state.LastReferenceAction.Type != "contest" {
		t.Fatalf("expected post-commit reference action, got %+v", state.LastReferenceAction)
	}
	if state.Status == "active" && len(state.ReferenceOutcomes) != 0 {
		t.Fatal("full rollouts should remain hidden until the scenario ends")
	}
}

func TestBestCaseLineIsExposedAtEndWithReasonedTurns(t *testing.T) {
	moment := testMoment()
	e := New(moment, "a")
	state := e.State()
	for state.Status == "active" {
		var err error
		state, err = e.Apply(model.Action{Type: "hold"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.BestCase == nil || len(state.BestCase.Steps) == 0 {
		t.Fatalf("expected a terminal best-case line, got %+v", state.BestCase)
	}
	for _, step := range state.BestCase.Steps {
		if step.Reason == "" || len(step.Alternatives) != len(actionTypes) {
			t.Fatalf("expected a reason and every modeled alternative at turn %d: %+v", step.Turn, step)
		}
	}

	replay := newEngine(moment, "best-case-replay", false, nil)
	for _, step := range state.BestCase.Steps {
		if _, err := replay.Apply(step.Action); err != nil {
			t.Fatal(err)
		}
	}
	if replay.session.Status != state.BestCase.Status || replay.session.OutcomeReason != state.BestCase.OutcomeReason {
		t.Fatalf("best-case steps did not reproduce their declared outcome: %+v vs %+v", replay.session, state.BestCase)
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

func TestMoveClampsTargetBeyondClassLimit(t *testing.T) {
	e := New(testMoment(), "a")
	after, err := e.Apply(model.Action{Type: "move", Target: &model.Point{X: 100, Y: 50}})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionUnit(t, after, "blue-carry").Position; got != (model.Point{X: 41, Y: 50}) {
		t.Fatalf("expected marksman movement to clamp at 11 units, got %+v", got)
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
			profile, _ := model.Profile(test.class)
			moment.Units[0] = model.Unit{
				ID: "blue-carry", Team: "blue", Role: string(test.class), Class: test.class,
				Position: model.Point{X: 30, Y: 50}, HP: profile.MaxHP, MaxHP: profile.MaxHP,
				AttackDamage: 20, Armor: 12, VisionRange: 34, AttackCooldown: 2, Alive: true,
			}
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

func TestDodgeLogsEvadedSkillshot(t *testing.T) {
	moment := testMoment()
	moment.Units[1].Position = model.Point{X: 38, Y: 50}
	state, err := New(moment, "a").Apply(model.Action{Type: "dodge"})
	if err != nil {
		t.Fatal(err)
	}
	if !logContains(state, "dodged red fighter's skillshot") {
		t.Fatalf("expected dodge log, got %+v", state.Log)
	}
	if sessionUnit(t, state, "blue-carry").HP != 70 {
		t.Fatal("a successfully dodged skillshot dealt damage")
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
		if !logContains(state, "outplayed red fighter") || sessionUnit(t, state, "red-one").HP >= 80 {
			t.Fatalf("expected successful outplay, got log=%+v", state.Log)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		moment := testMoment()
		moment.Units[1].Position = model.Point{X: 90, Y: 50}
		state, err := New(moment, "a").Apply(model.Action{Type: "outplay"})
		if err != nil {
			t.Fatal(err)
		}
		if !logContains(state, "outplay was unavailable") {
			t.Fatalf("failed outplay was reported incorrectly: %+v", state.Log)
		}
	})
}
