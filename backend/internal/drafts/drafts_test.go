package drafts

import (
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/highlight"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const objectiveDetection = `{"schemaVersion":"1.0","startSecond":742,"endSecond":754,"score":0.83,"reasonTags":["objective-steal","one-versus-many"],"signals":{"winProbabilitySwing":0.88,"eventDensity":0.81,"entityProximity":0.18,"resourceAsymmetry":0.72},"semanticEvidence":{"oneVersusManyUnitIds":["blue-carry"],"successfulEscapeUnitIds":[],"teamFightReversalSecond":null}}`

func TestCategoryForReasonTags(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{"reversal takes priority", []string{"successful-escape", "team-fight-reversal"}, "team-fight-engagement"},
		{"escape outranks generic proximity", []string{"team-fight", "successful-escape"}, "escape"},
		{"objective outranks generic proximity", []string{"team-fight", "objective-steal"}, "objective-contest"},
		{"vision outranks generic proximity", []string{"team-fight", "fog-collapse"}, "vision-uncertainty"},
		{"resource outranks generic proximity", []string{"team-fight", "resource-swing"}, "resource-trade"},
		{"isolation outranks generic proximity", []string{"team-fight", "one-versus-many"}, "positioning"},
		{"generic team fight", []string{"team-fight"}, "team-fight-engagement"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CategoryFor(test.tags); got != test.want {
				t.Fatalf("CategoryFor(%v) = %q, want %q", test.tags, got, test.want)
			}
		})
	}
}

func TestFromNDJSONPreservesDetectionAndLeavesAuthorshipIncomplete(t *testing.T) {
	bundle, err := FromNDJSON(strings.NewReader(objectiveDetection + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Version != "2.1" || len(bundle.Drafts) != 1 {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
	draft := bundle.Drafts[0]
	if draft.Scenario.StartTimeSeconds != 742 || draft.Scenario.SourceDetection == nil || draft.Scenario.SourceDetection.EndSecond != 754 {
		t.Fatalf("timestamps were not preserved: %+v", draft.Scenario.SourceDetection)
	}
	if draft.Scenario.SourceDetection.Score != 0.83 || draft.Scenario.Signals.WinProbabilitySwing != 0.88 {
		t.Fatalf("score or signals were not preserved: %+v", draft.Scenario.SourceDetection)
	}
	if got := draft.Scenario.SourceDetection.SemanticEvidence.OneVersusManyUnitIDs; len(got) != 1 || got[0] != "blue-carry" {
		t.Fatalf("semantic evidence was not preserved: %v", got)
	}
	if draft.Scenario.Authoring.Category != "objective-contest" || draft.Scenario.Authoring.AnalystRationale != "" || len(draft.Scenario.Authoring.IntendedTradeoffs) != 0 {
		t.Fatalf("draft authorship was not intentionally incomplete: %+v", draft.Scenario.Authoring)
	}
}

func TestFromNDJSONRejectsUnknownFieldsAndScoreMismatch(t *testing.T) {
	unknown := strings.Replace(objectiveDetection, `"schemaVersion":"1.0"`, `"schemaVersion":"1.0","unknown":true`, 1)
	if _, err := FromNDJSON(strings.NewReader(unknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
	mismatch := strings.Replace(objectiveDetection, `"score":0.83`, `"score":0.82`, 1)
	if _, err := FromNDJSON(strings.NewReader(mismatch)); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected score mismatch rejection, got %v", err)
	}
}

func TestIncompleteDraftCannotPublish(t *testing.T) {
	bundle, err := FromNDJSON(strings.NewReader(objectiveDetection))
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateDraft(bundle.Drafts[0])
	if err == nil {
		t.Fatal("expected intentionally incomplete draft to fail validation")
	}
	for _, message := range []string{"analyst rationale is incomplete", "intended tradeoffs are incomplete", "plausible alternatives are incomplete", "acceptance tests are incomplete"} {
		if !strings.Contains(err.Error(), message) {
			t.Fatalf("validation did not report %q: %v", message, err)
		}
	}
	if _, err := PreparePack(bundle.Drafts[0], nil); err == nil || !strings.Contains(err.Error(), "not publishable") {
		t.Fatalf("publish did not refuse incomplete draft: %v", err)
	}
}

func TestCompletedDraftCanJoinValidatedPack(t *testing.T) {
	moments, err := fixtures.Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	scenario := moments[0]
	end := scenario.StartTimeSeconds + 12
	scenario.SourceDetection = &model.TelemetryDetection{
		SchemaVersion: "1.0",
		StartSecond:   scenario.StartTimeSeconds,
		EndSecond:     end,
		Score:         highlight.RoundedScore(scenario.Signals),
		ReasonTags:    append([]string(nil), scenario.ReasonTags...),
		Signals:       scenario.Signals,
		SemanticEvidence: model.TelemetrySemanticEvidence{
			OneVersusManyUnitIDs:    []string{},
			SuccessfulEscapeUnitIDs: []string{},
		},
	}
	draft := Draft{Status: DraftStatus, Scenario: scenario}
	if err := ValidateDraft(draft); err != nil {
		t.Fatalf("completed draft did not validate: %v", err)
	}
	pack, err := PreparePack(draft, moments[1:])
	if err != nil {
		t.Fatalf("completed draft did not publish: %v", err)
	}
	if len(pack) != len(moments) || pack[len(pack)-1].SourceDetection == nil {
		t.Fatalf("published pack did not preserve provenance")
	}
}
