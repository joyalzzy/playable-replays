package telemetry

import (
	"errors"
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestServiceRunsCollectorToGuardedDraftJourney(t *testing.T) {
	service := NewService()
	if _, err := service.Start(model.CreateTelemetryMatchRequest{Source: "synthetic"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing consent should be rejected, got %v", err)
	}
	created, err := service.Start(model.CreateTelemetryMatchRequest{Source: "synthetic", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.CollectorToken == "" || created.Match.Status != "capturing" {
		t.Fatalf("unexpected start response: %+v", created)
	}

	for second := 0; second <= 12; second++ {
		batch := model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: second, Frames: []model.LiveTelemetryFrame{reversalFrame(second)}}
		if _, err := service.Ingest(created.Match.ID, created.CollectorToken, batch); err != nil {
			t.Fatalf("ingest second %d: %v", second, err)
		}
	}
	finalized, err := service.Finish(created.Match.ID, created.CollectorToken)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Status != "finalized" || len(finalized.Candidates) != 1 || finalized.Candidates[0].Status != "final" {
		t.Fatalf("unexpected finalized match: %+v", finalized)
	}

	result, err := service.Draft(finalized.ID, finalized.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || result.Bundle.Version != "2.1" || len(result.Bundle.Drafts) != 1 {
		t.Fatalf("unexpected guarded draft: %+v", result)
	}
	joined := strings.Join(result.CompletionIssues, " ")
	for _, required := range []string{"rationale", "tradeoffs", "alternatives", "acceptance tests"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing analyst gate %q in %v", required, result.CompletionIssues)
		}
	}
	if result.Bundle.Drafts[0].Scenario.SourceDetection == nil || result.Bundle.Drafts[0].Scenario.SourceDetection.StartSecond != 0 {
		t.Fatalf("detection evidence was not preserved: %+v", result.Bundle.Drafts[0].Scenario.SourceDetection)
	}
}

func TestRejectedBatchDoesNotMutateMatch(t *testing.T) {
	service := NewService()
	created, err := service.Start(model.CreateTelemetryMatchRequest{Source: "authorized", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	first := model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: 0, Frames: []model.LiveTelemetryFrame{basicFrame(0, 0.5)}}
	if _, err := service.Ingest(created.Match.ID, "wrong-token", first); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong token should be rejected, got %v", err)
	}
	invalid := model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: 0, Frames: []model.LiveTelemetryFrame{basicFrame(0, 0.5), basicFrame(0, 0.6)}}
	if _, err := service.Ingest(created.Match.ID, created.CollectorToken, invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid batch should be rejected, got %v", err)
	}
	current, err := service.Get(created.Match.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.FrameCount != 0 || current.ExpectedSequence != 0 || current.LastSecond != -1 {
		t.Fatalf("rejected input mutated state: %+v", current)
	}
	timeline, err := service.Timeline(created.Match.ID)
	if err != nil {
		t.Fatal(err)
	}
	if timeline.SourceFrameCount != 0 || len(timeline.Frames) != 0 || len(timeline.Events) != 0 {
		t.Fatalf("rejected input mutated timeline: %+v", timeline)
	}
	if _, err := service.Ingest(created.Match.ID, created.CollectorToken, first); err != nil {
		t.Fatalf("valid retry should still work: %v", err)
	}
}

func TestReadDocumentIsStrictAndNormalized(t *testing.T) {
	valid := `{"version":"1.0","frames":[{"second":0,"winProbability":0.5,"events":[],"units":[{"id":"blue","team":"blue","position":{"x":10,"y":10},"hp":100,"maxHp":100,"gold":0,"alive":true},{"id":"red","team":"red","position":{"x":20,"y":20},"hp":100,"maxHp":100,"gold":0,"alive":true}]},{"second":1,"winProbability":0.6,"events":["damage"],"units":[{"id":"blue","team":"blue","position":{"x":10,"y":10},"hp":90,"maxHp":100,"gold":1,"alive":true},{"id":"red","team":"red","position":{"x":20,"y":20},"hp":90,"maxHp":100,"gold":1,"alive":true}]}]}`
	if _, err := ReadDocument(strings.NewReader(valid)); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	unknown := strings.Replace(valid, `"version":"1.0"`, `"version":"1.0","playerName":"private"`, 1)
	if _, err := ReadDocument(strings.NewReader(unknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("identity-bearing unknown input should be rejected, got %v", err)
	}
}
