package fixtures

import "testing"

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
	for _, moment := range moments {
		categoryCounts[moment.Authoring.Category]++
		levels[moment.Authoring.SkillLevel]++
		if len(moment.Rules.ReferencePlan) != moment.MaxTurns || len(moment.Rules.ReferenceReasons) != moment.MaxTurns {
			t.Fatalf("moment %q is missing a full authored reference line", moment.ID)
		}
		if len(moment.Authoring.IntendedTradeoffs) < 2 || len(moment.Authoring.PlausibleAlternatives) < 2 || len(moment.Authoring.AcceptanceTests) < 2 {
			t.Fatalf("moment %q is missing authoring evidence", moment.ID)
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

func TestRejectsIncompleteAuthoringMetadata(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	moment := moments[0]
	moment.Authoring.AnalystRationale = ""
	if err := validateMoment(moment); err == nil {
		t.Fatal("expected blank analyst rationale to be rejected")
	}
}
