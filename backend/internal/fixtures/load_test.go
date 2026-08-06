package fixtures

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/highlight"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestAuthoredScenarioPackCoverage(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(moments) != 12 {
		t.Fatalf("expected twelve moments, got %d", len(moments))
	}
	categoryCounts := map[string]int{}
	levels := map[string]int{}
	mechanicBriefings := 0
	for _, moment := range moments {
		categoryCounts[moment.Authoring.Category]++
		levels[moment.Authoring.SkillLevel]++
		if len(moment.Rules.ReferencePlan) != moment.MaxTurns || len(moment.Rules.ReferenceReasons) != moment.MaxTurns {
			t.Fatalf("moment %q is missing a full authored reference line", moment.ID)
		}
		if len(moment.Authoring.IntendedTradeoffs) < 2 || len(moment.Authoring.PlausibleAlternatives) < 2 || len(moment.Authoring.AcceptanceTests) < 2 {
			t.Fatalf("moment %q is missing authoring evidence", moment.ID)
		}
		if moment.MechanicBriefing != nil {
			mechanicBriefings++
		}
	}
	for _, category := range categories {
		if categoryCounts[category] != 2 {
			t.Fatalf("expected two %q scenarios, got %d", category, categoryCounts[category])
		}
	}
	for _, level := range skillLevels {
		if levels[level] == 0 {
			t.Fatalf("skill level %q is not covered", level)
		}
	}
	if mechanicBriefings != 6 {
		t.Fatalf("expected six scenario-specific mechanic briefings, got %d", mechanicBriefings)
	}
}

func TestScenarioSpecificMechanicsRequireCompleteBriefing(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := findMoment(t, moments, "objective-contest-318")
	moment.MechanicBriefing.Mechanics = moment.MechanicBriefing.Mechanics[:1]
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected an unexplained shrine ring to be rejected")
	}

	moments, err = Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment = findMoment(t, moments, "vision-uncertainty-356")
	moment.MechanicBriefing.Mechanics[0].RoleInScenario = ""
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected an incomplete mechanic explanation to be rejected")
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

func TestCanonicalTerrainLandmarksCannotMove(t *testing.T) {
	tests := []struct {
		momentID  string
		terrainID string
	}{
		{momentID: "objective-steal-742", terrainID: "river"},
		{momentID: "teamfight-reversal-1091", terrainID: "base-gate"},
		{momentID: "escape-412", terrainID: "tower-zone"},
		{momentID: "escape-1260", terrainID: "exit-zone"},
		{momentID: "vision-uncertainty-356", terrainID: "lane-pocket"},
		{momentID: "vision-uncertainty-1004", terrainID: "exit-pocket"},
	}

	for _, test := range tests {
		t.Run(test.terrainID, func(t *testing.T) {
			moments, err := Load("../../../fixtures/moments.json")
			if err != nil {
				t.Fatal(err)
			}
			moment := findMoment(t, moments, test.momentID)
			for i := range moment.Rules.Terrain {
				if moment.Rules.Terrain[i].ID == test.terrainID {
					moment.Rules.Terrain[i].Position.X++
					if err := ValidateMoment(moment); err == nil {
						t.Fatalf("expected moved %q terrain to be rejected", test.terrainID)
					}
					return
				}
			}
			t.Fatalf("moment %q does not contain terrain %q", test.momentID, test.terrainID)
		})
	}
}

func TestCanonicalSafeZoneMustMatchLandmark(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := findMoment(t, moments, "escape-412")
	moment.Rules.Victory.SafeZone.X++
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected a safe zone moved away from its canonical landmark to be rejected")
	}
}

func TestRiverCoreCannotLeaveCanonicalRiver(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := findMoment(t, moments, "objective-steal-742")
	moment.Rules.Objective.Position.Y++
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected a moved river core to be rejected")
	}

	moments, err = Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment = findMoment(t, moments, "objective-steal-742")
	terrain := moment.Rules.Terrain[:0]
	for _, feature := range moment.Rules.Terrain {
		if feature.ID != "river" {
			terrain = append(terrain, feature)
		}
	}
	moment.Rules.Terrain = terrain
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected a river core without canonical river terrain to be rejected")
	}
}

func TestRejectsIncompleteAuthoringMetadata(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := moments[0]
	moment.Authoring.AnalystRationale = ""
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected blank analyst rationale to be rejected")
	}
}

func TestSourceDetectionMustRemainConsistent(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := moments[0]
	moment.SourceDetection = &model.TelemetryDetection{
		SchemaVersion: "1.0",
		StartSecond:   moment.StartTimeSeconds,
		EndSecond:     moment.StartTimeSeconds + 12,
		Score:         highlight.RoundedScore(moment.Signals),
		ReasonTags:    append([]string(nil), moment.ReasonTags...),
		Signals:       moment.Signals,
		SemanticEvidence: model.TelemetrySemanticEvidence{
			OneVersusManyUnitIDs:    []string{"blue-carry"},
			SuccessfulEscapeUnitIDs: []string{},
		},
	}
	if err := ValidateMoment(moment); err != nil {
		t.Fatalf("valid source detection was rejected: %v", err)
	}
	moment.SourceDetection.Score = 0
	if err := ValidateMoment(moment); err == nil {
		t.Fatal("expected inconsistent detector score to be rejected")
	}
}

func TestRejectsInvalidPlayableUnitState(t *testing.T) {
	tests := map[string]func(*model.Moment){
		"unknown class": func(moment *model.Moment) {
			moment.Units[0].Class = "wizard"
		},
		"class health": func(moment *model.Moment) {
			moment.Units[0].MaxHP++
		},
		"health overflow": func(moment *model.Moment) {
			moment.Units[0].HP = moment.Units[0].MaxHP + 1
		},
		"alive mismatch": func(moment *model.Moment) {
			moment.Units[0].Alive = !moment.Units[0].Alive
		},
		"invalid team": func(moment *model.Moment) {
			moment.Units[0].Team = "green"
		},
		"negative cooldown": func(moment *model.Moment) {
			moment.Units[0].Cooldown = -1
		},
		"outside map": func(moment *model.Moment) {
			moment.Units[0].Position.X = 101
		},
		"duplicate id": func(moment *model.Moment) {
			moment.Units[1].ID = moment.Units[0].ID
		},
		"missing controlled": func(moment *model.Moment) {
			moment.ControlledUnitID = "missing"
		},
		"empty role": func(moment *model.Moment) {
			moment.Units[0].Role = ""
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			moments, err := Load("../../../fixtures/moments.json")
			if err != nil {
				t.Fatal(err)
			}
			moment := moments[0]
			mutate(&moment)
			if err := ValidateMoment(moment); err == nil {
				t.Fatal("expected invalid unit state to be rejected")
			}
		})
	}
}

func TestRejectsMoreThanSnapshotLimit(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := moments[0]
	template := moment.Units[1]
	for len(moment.Units) <= maxUnitsPerMoment {
		unit := template
		unit.ID = fmt.Sprintf("extra-unit-%d", len(moment.Units))
		moment.Units = append(moment.Units, unit)
	}
	if err := ValidateMoment(moment); err == nil {
		t.Fatalf("expected a %d-unit moment to be rejected", len(moment.Units))
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
				t.Fatal("expected invalid JSON fixture to be rejected")
			}
		})
	}
}

func findMoment(t *testing.T, moments []model.Moment, id string) model.Moment {
	t.Helper()
	for _, moment := range moments {
		if moment.ID == id {
			return moment
		}
	}
	t.Fatalf("moment %q was not found", id)
	return model.Moment{}
}
