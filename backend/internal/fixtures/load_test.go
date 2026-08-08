package fixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestReducedReplayScenarioPack(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(moments) != 3 {
		t.Fatalf("expected three focused scenarios, got %d", len(moments))
	}

	expectedIDs := []string{"resource-trade-932", "positioning-1295", "teamfight-reversal-1727"}
	for index, moment := range moments {
		if moment.ID != expectedIDs[index] {
			t.Fatalf("scenario %d has id %q, want %q", index, moment.ID, expectedIDs[index])
		}
		if len(moment.Units) != 10 {
			t.Fatalf("moment %q does not contain a full 5v5 roster", moment.ID)
		}
		teamCounts := map[string]int{}
		marksmen := map[string]int{}
		for _, unit := range moment.Units {
			teamCounts[unit.Team]++
			if unit.Class == model.ClassMarksman {
				marksmen[unit.Team]++
			}
		}
		if teamCounts["blue"] != 5 || teamCounts["red"] != 5 {
			t.Fatalf("moment %q has team counts %+v", moment.ID, teamCounts)
		}
		if marksmen["blue"] != 1 || marksmen["red"] != 1 {
			t.Fatalf("moment %q has marksman counts %+v", moment.ID, marksmen)
		}
		if moment.ReplayEvidence == nil || !strings.Contains(strings.ToLower(moment.ReplayEvidence.CoordinateMethod), "approx") {
			t.Fatalf("moment %q does not disclose approximate coordinates", moment.ID)
		}
		if len(moment.Rules.ActionDefaults) != len(actionTypes) {
			t.Fatalf("moment %q exposes %d action defaults", moment.ID, len(moment.Rules.ActionDefaults))
		}
		if len(moment.Rules.ReferencePlan) != moment.MaxTurns || len(moment.Rules.ReferenceReasons) != moment.MaxTurns {
			t.Fatalf("moment %q has an incomplete reference plan", moment.ID)
		}
		assertNoObsoleteActions(t, moment)
	}
}

func TestAuthoredAcceptanceCases(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAcceptance(moments); err != nil {
		t.Fatal(err)
	}
}

func TestPackBoundsAreOneToThree(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLibrary(moments[:1]); err != nil {
		t.Fatalf("one scenario should be valid: %v", err)
	}
	tooMany := append(append([]model.Moment(nil), moments...), moments[0])
	tooMany[3].ID = "fourth-scenario"
	tooMany[3].Slug = "fourth-scenario"
	if err := ValidateLibrary(tooMany); err == nil {
		t.Fatal("expected four scenarios to be rejected")
	}
}

func TestReplayEvidenceIsRequired(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := moments[0]
	moment.ReplayEvidence = nil
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected missing replay evidence to be rejected")
	}

	moment = moments[0]
	moment.ReplayEvidence.CoordinateMethod = "Exact telemetry coordinates"
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected coordinates without approximation disclosure to be rejected")
	}
}

func TestLoadRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	source, err := os.ReadFile("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(source, &document); err != nil {
		t.Fatal(err)
	}
	document["admin"] = true
	unknownField, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	for name, body := range map[string][]byte{
		"unknown":  unknownField,
		"trailing": append(append([]byte(nil), source...), []byte(" {}")...),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "moments.json")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid fixture JSON to be rejected")
			}
		})
	}
}

func assertNoObsoleteActions(t *testing.T, moment model.Moment) {
	t.Helper()
	check := func(action model.Action) {
		if action.Type == "dodge" || action.Type == "outplay" {
			t.Fatalf("moment %q contains obsolete tactical action %q", moment.ID, action.Type)
		}
	}
	for _, action := range moment.Rules.ReferencePlan {
		check(action)
	}
	for _, action := range moment.Rules.ActionDefaults {
		check(action)
	}
	for _, actions := range moment.Rules.ReferenceContinuations {
		for _, action := range actions {
			check(action)
		}
	}
	for _, test := range moment.Authoring.AcceptanceTests {
		for _, action := range test.Actions {
			check(action)
		}
	}
}
