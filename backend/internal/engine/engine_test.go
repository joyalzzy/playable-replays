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

func TestMarksmanProjectilePersistsUntilNextTacticalTurn(t *testing.T) {
	e := New(testMoment(), "a")
	state, err := e.Apply(model.Action{Type: "contest"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionUnit(t, state, "red-one").HP; got != 80 {
		t.Fatalf("pending projectile dealt damage immediately: red HP=%d", got)
	}
	if len(state.Projectiles) != 1 || state.Projectiles[0].Team != "blue" ||
		state.Projectiles[0].SourceUnitID != "blue-carry" || state.Projectiles[0].TargetUnitID != "red-one" ||
		state.Projectiles[0].Damage != 63 {
		t.Fatalf("expected a public half-health marksman projectile, got %+v", state.Projectiles)
	}
	if !logContains(state, "potential damage") {
		t.Fatalf("projectile launch was not explained: %+v", state.Log)
	}

	state, err = e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Projectiles) != 0 || sessionUnit(t, state, "red-one").HP != 17 {
		t.Fatalf("projectile did not resolve at the next turn start: %+v", state)
	}
	if !logContains(state, "projectile hit") {
		t.Fatalf("projectile impact was not explained: %+v", state.Log)
	}
}

func TestTargetedContestAttacksSelectedEnemyInRange(t *testing.T) {
	moment := testMoment()
	moment.Units = append(moment.Units, model.Unit{
		ID: "red-nearer", Team: "red", Role: "support", Class: model.ClassSupport, Policy: "support",
		Position: model.Point{X: 38, Y: 50}, HP: 110, MaxHP: 110, Alive: true,
	})
	e := New(moment, "a")
	state, err := e.ApplyTargetedContext(context.Background(), model.Action{Type: "contest"}, "red-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Projectiles) != 1 || state.Projectiles[0].SourceUnitID != "blue-carry" ||
		state.Projectiles[0].TargetUnitID != "red-one" {
		t.Fatalf("targeted contest did not attack the selected enemy: %+v", state.Projectiles)
	}
}

func TestTargetedContestCanKeepPressureWhileAttackCoolsDown(t *testing.T) {
	moment := testMoment()
	moment.MaxTurns = 4
	moment.Units[1].HP = 125
	moment.Units = append(moment.Units,
		model.Unit{ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Policy: "support", Position: model.Point{X: 10, Y: 10}, HP: 110, MaxHP: 110, Alive: true},
		model.Unit{ID: "red-support", Team: "red", Role: "support", Class: model.ClassSupport, Policy: "support", Position: model.Point{X: 90, Y: 90}, HP: 110, MaxHP: 110, Alive: true},
	)
	e := New(moment, "a")

	state, err := e.ApplyTargetedContext(context.Background(), model.Action{Type: "contest"}, "red-one")
	if err != nil || len(state.Projectiles) != 1 {
		t.Fatalf("first targeted contest did not fire: err=%v state=%+v", err, state)
	}
	state, err = e.ApplyTargetedContext(context.Background(), model.Action{Type: "contest"}, "red-one")
	if err != nil {
		t.Fatalf("contest should remain legal while the attack cools down: %v", err)
	}
	if len(state.Projectiles) != 0 || sessionUnit(t, state, "blue-carry").Cooldown != 1 {
		t.Fatalf("cooldown contest should maintain pressure without firing early: %+v", state)
	}
	state, err = e.ApplyTargetedContext(context.Background(), model.Action{Type: "contest"}, "red-one")
	if err != nil {
		t.Fatalf("targeted contest did not become ready again: %v", err)
	}
	if len(state.Projectiles) != 1 || state.Projectiles[0].SourceUnitID != "blue-carry" ||
		state.Projectiles[0].TargetUnitID != "red-one" {
		t.Fatalf("targeted contest did not fire again after cooldown: %+v", state.Projectiles)
	}
}

func TestInvalidTargetedContestLeavesStateUnchanged(t *testing.T) {
	tests := map[string]func(*model.Moment) string{
		"allied target":  func(moment *model.Moment) string { return "blue-carry" },
		"unknown target": func(moment *model.Moment) string { return "missing" },
		"hidden target": func(moment *model.Moment) string {
			moment.Units[1].Position = model.Point{X: 99, Y: 99}
			return "red-one"
		},
		"defeated target": func(moment *model.Moment) string {
			moment.Units[1].HP = 0
			moment.Units[1].Alive = false
			return "red-one"
		},
		"outside attack range": func(moment *model.Moment) string {
			moment.Units[0].VisionRange = 100
			moment.Units[1].Position = model.Point{X: 70, Y: 50}
			return "red-one"
		},
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			moment := testMoment()
			targetUnitID := configure(&moment)
			e := New(moment, "a")
			before := e.State()
			after, err := e.ApplyTargetedContext(context.Background(), model.Action{Type: "contest"}, targetUnitID)
			if !errors.Is(err, ErrIllegalAction) {
				t.Fatalf("expected illegal action, got %v", err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("invalid targeted contest mutated state")
			}
		})
	}
}

func TestPlayerProjectileUsesTwoChargesWithoutAdvancingTurn(t *testing.T) {
	moment := testMoment()
	moment.Units[1].HP = 125
	e := New(moment, "a")
	initial := e.State()
	if initial.ProjectileCharges != 2 || !initial.ProjectileAvailable {
		t.Fatalf("expected two ready projectile charges: %+v", initial)
	}
	state, err := e.FireProjectile("blue-carry", "red-one")
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != 0 || state.ProjectileCharges != 1 || state.ProjectileAvailable || len(state.Projectiles) != 1 ||
		state.Projectiles[0].SourceUnitID != "blue-carry" || state.Projectiles[0].TargetUnitID != "red-one" {
		t.Fatalf("first player projectile was not queued as a bounded reaction: %+v", state)
	}

	if _, err = e.Apply(model.Action{Type: "hold"}); err != nil {
		t.Fatal(err)
	}
	if state, err = e.Apply(model.Action{Type: "hold"}); err != nil {
		t.Fatal(err)
	}
	if !state.ProjectileAvailable {
		t.Fatalf("marksman should be ready to spend the second charge: %+v", state)
	}
	state, err = e.FireProjectile("blue-carry", "red-one")
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != 2 || state.ProjectileCharges != 0 || state.ProjectileAvailable || len(state.Projectiles) != 1 {
		t.Fatalf("second projectile did not exhaust the player charges: %+v", state)
	}
	before := state
	after, err := e.FireProjectile("blue-carry", "red-one")
	if !errors.Is(err, ErrProjectileUnavailable) || !reflect.DeepEqual(before, after) {
		t.Fatalf("exhausted projectile fire should fail without mutation: err=%v state=%+v", err, after)
	}
}

func TestNonMarksmanPlayerCanDirectMarksmanTeammate(t *testing.T) {
	moment := testMoment()
	moment.Units[0] = model.Unit{
		ID: "blue-carry", Team: "blue", Role: "fighter", Class: model.ClassFighter, Policy: "controlled",
		Position: model.Point{X: 30, Y: 50}, HP: 125, MaxHP: 125, Alive: true,
	}
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-marksman", Team: "blue", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 32, Y: 50}, HP: 90, MaxHP: 90, Alive: true,
	})
	e := New(moment, "a")
	state, err := e.FireProjectile("blue-marksman", "red-one")
	if err != nil || len(state.Projectiles) != 1 || state.Projectiles[0].SourceUnitID != "blue-marksman" {
		t.Fatalf("marksman teammate did not fire for a non-marksman player: err=%v state=%+v", err, state)
	}
}

func TestMarksmanPlayerCannotSpendChargeThroughAnotherMarksman(t *testing.T) {
	moment := testMoment()
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-other-marksman", Team: "blue", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 32, Y: 50}, HP: 90, MaxHP: 90, Alive: true,
	})
	e := New(moment, "a")
	before := e.State()
	after, err := e.FireProjectile("blue-other-marksman", "red-one")
	if !errors.Is(err, ErrProjectileUnavailable) || !reflect.DeepEqual(before, after) {
		t.Fatalf("marksman player should be the charged projectile source: err=%v state=%+v", err, after)
	}
}

func TestNonMarksmanCannotCreateProjectile(t *testing.T) {
	moment := testMoment()
	moment.Units[0].Role = "fighter"
	moment.Units[0].Class = model.ClassFighter
	e := New(moment, "a")
	source := e.unit("blue-carry")
	target := e.unit("red-one")
	before := e.State()

	if e.fireProjectile(source, target, "user") {
		t.Fatal("non-marksman unexpectedly created a projectile")
	}
	if after := e.State(); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected non-marksman projectile mutated state: before=%+v after=%+v", before, after)
	}
}

func TestTeamTotalHealthThresholdDeterminesOutcome(t *testing.T) {
	tests := []struct {
		name           string
		playerHealth   int
		opponentHealth int
		wantStatus     string
	}{
		{name: "exact player two-to-one lead wins", playerHealth: 80, opponentHealth: 40, wantStatus: "won"},
		{name: "exact opponent two-to-one lead loses", playerHealth: 50, opponentHealth: 100, wantStatus: "lost"},
		{name: "sub-threshold state remains active", playerHealth: 79, opponentHealth: 40, wantStatus: "active"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := newEngine(testMoment(), "a", false, nil)
			e.unit("blue-carry").HP = test.playerHealth
			e.unit("blue-carry").Alive = test.playerHealth > 0
			e.unit("red-one").HP = test.opponentHealth
			e.unit("red-one").Alive = test.opponentHealth > 0

			e.evaluateOutcome()

			if e.session.Status != test.wantStatus {
				t.Fatalf("expected %s at %d:%d health, got %s: %s", test.wantStatus,
					test.playerHealth, test.opponentHealth, e.session.Status, e.session.OutcomeReason)
			}
		})
	}
}

func TestScenarioContinuesPastAuthoredHorizonUntilHealthThreshold(t *testing.T) {
	moment := testMoment()
	moment.MaxTurns = 1
	e := newEngine(moment, "a", false, nil)

	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil || state.Status != "active" || state.Turn != 1 {
		t.Fatalf("authored horizon unexpectedly ended the scenario: err=%v state=%+v", err, state)
	}
	state, err = e.Apply(model.Action{Type: "hold"})
	if err != nil || state.Status != "active" || state.Turn != 2 {
		t.Fatalf("scenario did not continue beyond its authored horizon: err=%v state=%+v", err, state)
	}
}

func TestSecuringObjectiveDoesNotBypassHealthThreshold(t *testing.T) {
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
	if state.Status != "active" || state.Objective == nil || state.Objective.Status != "secured-blue" || state.OutcomeReason != "" {
		t.Fatalf("objective control should remain tactical state until the health threshold is reached: %+v", state)
	}
}

func TestControlTransfersAfterControlledUnitDiesWithoutHealthDeficit(t *testing.T) {
	moment := testMoment()
	moment.Units[0].HP = 40
	moment.Units[0].VisionRange = 10
	moment.Units[1] = model.Unit{
		ID: "red-marksman", Team: "red", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 48, Y: 50}, HP: 90, MaxHP: 90, Alive: true,
	}
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Policy: "support",
		Position: model.Point{X: 25, Y: 50}, HP: 110, MaxHP: 110, Alive: true,
	})
	e := newEngine(moment, "a", false, nil)
	if _, err := e.Apply(model.Action{Type: "hold"}); err != nil {
		t.Fatal(err)
	}
	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "active" || state.ControlledUnitID != "blue-support" || !logContains(state, "Control transferred") {
		t.Fatalf("play did not transfer to a surviving teammate: %+v", state)
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

func TestReferenceProjectionsExplainUnresolvedAuthoredHorizon(t *testing.T) {
	moment := testMoment()
	moment.MaxTurns = 1
	e := New(moment, "a")

	for _, outcome := range e.referenceOutcomes {
		if outcome.Status != "active" || !strings.Contains(outcome.OutcomeReason, "Neither team reached the 2:1") {
			t.Fatalf("unresolved reference outcome was not explained: %+v", outcome)
		}
	}
	if e.bestCase == nil || e.bestCase.Status != "active" || !strings.Contains(e.bestCase.OutcomeReason, "Neither team reached the 2:1") {
		t.Fatalf("unresolved best-case horizon was not explained: %+v", e.bestCase)
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

func TestFullMapHasThreeTurretsPerSide(t *testing.T) {
	e := New(testMoment(), "a")
	state := e.State()
	if len(state.Turrets) != 6 {
		t.Fatalf("expected six full-map turrets, got %+v", state.Turrets)
	}
	counts := map[string]int{}
	lanes := map[string]map[string]bool{"blue": {}, "red": {}}
	for _, turret := range state.Turrets {
		counts[turret.Team]++
		lanes[turret.Team][turret.Lane] = true
		if turret.ID == "" || turret.HP != 3000 || turret.MaxHP != 3000 || !turret.Alive || !pointInBounds(turret.Position) {
			t.Fatalf("invalid canonical turret: %+v", turret)
		}
	}
	for _, team := range []string{"blue", "red"} {
		if counts[team] != 3 || !lanes[team]["top"] || !lanes[team]["middle"] || !lanes[team]["bottom"] {
			t.Fatalf("expected top/middle/bottom turrets for %s, got counts=%v lanes=%v", team, counts, lanes)
		}
	}
	state.Turrets[0].HP = 0
	if e.State().Turrets[0].HP != 3000 {
		t.Fatal("public turret state was not returned defensively")
	}
}

func TestDodgeEvadesTwoIncomingProjectilesWithoutAdvancingTurn(t *testing.T) {
	moment := testMoment()
	moment.Units[1] = model.Unit{
		ID: "red-one", Team: "red", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 48, Y: 50}, HP: 90, MaxHP: 90,
		AttackRange: 28, AttackDamage: 20, MoveRange: 11, MoveSpeed: 11,
		Armor: 12, VisionRange: 34, AttackCooldown: 1, Alive: true,
	}
	e := New(moment, "a")
	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != 1 || len(state.Projectiles) != 1 || !state.DodgeAvailable || state.DodgeCharges != 2 {
		t.Fatalf("expected one incoming projectile and two charges: %+v", state)
	}

	state, err = e.Dodge()
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != 1 || len(state.Projectiles) != 0 || state.DodgeAvailable || state.DodgeCharges != 1 ||
		sessionUnit(t, state, "blue-carry").HP != 70 {
		t.Fatalf("first Dodge did not consume only the incoming projectile and one charge: %+v", state)
	}
	if !logContains(state, "1 Dodge charge remain") {
		t.Fatalf("first Dodge was not explained: %+v", state.Log)
	}

	state, err = e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != 2 || len(state.Projectiles) != 1 || !state.DodgeAvailable {
		t.Fatalf("expected the marksman to fire again after its one-turn cooldown: %+v", state)
	}
	state, err = e.Dodge()
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != 2 || len(state.Projectiles) != 0 || state.DodgeAvailable || state.DodgeCharges != 0 ||
		sessionUnit(t, state, "blue-carry").HP != 70 {
		t.Fatalf("second Dodge did not consume the final charge: %+v", state)
	}

	before := state
	after, err := e.Dodge()
	if !errors.Is(err, ErrDodgeUnavailable) {
		t.Fatalf("expected exhausted Dodge to be unavailable, got %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unavailable Dodge mutated state")
	}
}

func TestDodgeRefreshesFogAfterReactionMovement(t *testing.T) {
	moment := testMoment()
	moment.Units[1] = model.Unit{
		ID: "red-marksman", Team: "red", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 48, Y: 50}, HP: 90, MaxHP: 90,
		AttackRange: 28, AttackDamage: 20, MoveRange: 11, MoveSpeed: 11,
		Armor: 12, VisionRange: 34, AttackCooldown: 2, Alive: true,
	}
	moment.Units = append(moment.Units, model.Unit{
		ID: "red-hidden", Team: "red", Role: "support", Class: model.ClassSupport, Policy: "support",
		Position: model.Point{X: 30, Y: 93}, HP: 110, MaxHP: 110,
		AttackRange: 20, AttackDamage: 10, MoveRange: 8, MoveSpeed: 8,
		Armor: 15, VisionRange: 30, AttackCooldown: 2, Alive: true,
	}, model.Unit{
		ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Policy: "support",
		Position: model.Point{X: 20, Y: 50}, HP: 110, MaxHP: 110, Alive: true,
	})
	e := New(moment, "a")
	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil || !state.DodgeAvailable || state.UnknownEnemyCount != 1 {
		t.Fatalf("expected a dodge window with one hidden enemy: err=%v state=%+v", err, state)
	}
	state, err = e.Dodge()
	if err != nil {
		t.Fatal(err)
	}
	if state.UnknownEnemyCount != 0 || state.VisibleEnemyCount != 2 || len(state.Units) != 4 {
		t.Fatalf("Dodge movement did not refresh public fog state: %+v", state)
	}
}

func TestHiddenProjectileSourceStaysRedactedOnImpact(t *testing.T) {
	moment := testMoment()
	moment.Units[0].HP = 40
	moment.Units[0].VisionRange = 10
	moment.Units[1] = model.Unit{
		ID: "red-hidden-marksman", Team: "red", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 48, Y: 50}, HP: 90, MaxHP: 90,
		AttackRange: 28, AttackDamage: 20, MoveRange: 11, MoveSpeed: 11,
		Armor: 12, VisionRange: 34, AttackCooldown: 2, Alive: true,
	}
	moment.Units = append(moment.Units, model.Unit{
		ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Policy: "support",
		Position: model.Point{X: 0, Y: 0}, HP: 40, MaxHP: 110, Alive: true,
	})
	e := New(moment, "a")
	state, err := e.Apply(model.Action{Type: "hold"})
	if err != nil || len(state.Projectiles) != 1 || state.Projectiles[0].SourceUnitID != "" {
		t.Fatalf("expected a source-redacted incoming projectile: err=%v state=%+v", err, state)
	}
	state, err = e.Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	foundElimination := false
	for _, entry := range state.Log {
		if (entry.Kind == "projectile" || entry.Kind == "elimination") &&
			entry.ActorID == "red-hidden-marksman" {
			t.Fatalf("hidden marksman ID leaked through projectile resolution: %+v", entry)
		}
		foundElimination = foundElimination || entry.Kind == "elimination"
	}
	if !foundElimination || state.Status != "lost" {
		t.Fatalf("expected the source-redacted projectile to exercise elimination: %+v", state)
	}
}

func TestHoldSynchronizesAggressiveAllyOnAuthoredTarget(t *testing.T) {
	moment := testMoment()
	moment.Rules.Victory.Kind = "eliminate-target"
	moment.Rules.Victory.TargetUnitID = "red-one"
	moment.Units = append(moment.Units,
		model.Unit{
			ID: "blue-marksman", Team: "blue", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
			Position: model.Point{X: 40, Y: 50}, HP: 90, MaxHP: 90, AttackDamage: 20,
			Armor: 12, VisionRange: 34, AttackCooldown: 2, Alive: true,
		},
		model.Unit{
			ID: "red-decoy", Team: "red", Role: "fighter", Class: model.ClassFighter, Policy: "protector",
			Position: model.Point{X: 42, Y: 50}, HP: 125, MaxHP: 125, AttackDamage: 16,
			Armor: 15, VisionRange: 32, AttackCooldown: 2, Alive: true,
		},
	)

	state, err := New(moment, "a").Apply(model.Action{Type: "hold"})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Projectiles) != 1 || state.Projectiles[0].SourceUnitID != "blue-marksman" ||
		state.Projectiles[0].TargetUnitID != "red-one" {
		t.Fatalf("Hold did not synchronize the aggressive ally onto the authored target: %+v", state.Projectiles)
	}
}

func TestLegacyDodgeAndOutplayAreNotTacticalActions(t *testing.T) {
	for _, actionType := range []string{"dodge", "outplay"} {
		t.Run(actionType, func(t *testing.T) {
			e := New(testMoment(), "a")
			before := e.State()
			after, err := e.Apply(model.Action{Type: actionType})
			if !errors.Is(err, ErrIllegalAction) {
				t.Fatalf("expected %q to be rejected, got %v", actionType, err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected %q action mutated state", actionType)
			}
		})
	}
}
