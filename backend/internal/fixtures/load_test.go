package fixtures

import (
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
