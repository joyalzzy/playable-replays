package telemetry

import (
	"slices"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestDetectorEmitsOnlyFullyCoveredAuditableWindow(t *testing.T) {
	var detector Detector
	for second := 0; second <= 12; second++ {
		candidates := detector.Add([]model.LiveTelemetryFrame{reversalFrame(second)})
		if second < 12 && len(candidates) != 0 {
			t.Fatalf("candidate appeared before its window was covered at second %d", second)
		}
	}
	candidates := detector.Candidates("provisional", nil)
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	candidate := candidates[0]
	if candidate.ID != "candidate-0-12" || candidate.Status != "provisional" || candidate.Category != "team-fight-engagement" {
		t.Fatalf("unexpected candidate summary: %+v", candidate)
	}
	if !slices.Contains(candidate.Detection.ReasonTags, "one-versus-many") || !slices.Contains(candidate.Detection.ReasonTags, "team-fight-reversal") {
		t.Fatalf("semantic evidence tags missing: %v", candidate.Detection.ReasonTags)
	}
	if candidate.Detection.SemanticEvidence.TeamFightReversalSecond == nil || *candidate.Detection.SemanticEvidence.TeamFightReversalSecond != 6 {
		t.Fatalf("unexpected reversal evidence: %+v", candidate.Detection.SemanticEvidence)
	}
}

func TestDetectorUsesCanonicalOverlapSuppression(t *testing.T) {
	var detector Detector
	frames := make([]model.LiveTelemetryFrame, 0, 25)
	for second := 0; second < 25; second++ {
		probability := 0.1
		if second == 12 {
			probability = 0.9
		}
		frame := basicFrame(second, probability)
		frame.Events = []string{"damage"}
		frames = append(frames, frame)
	}
	detector.Add(frames)
	candidates := detector.Candidates("final", nil)
	if len(candidates) != 3 || candidates[0].Detection.StartSecond != 0 || candidates[1].Detection.StartSecond != 6 || candidates[2].Detection.StartSecond != 12 {
		t.Fatalf("unexpected selected windows: %+v", candidates)
	}
}

func TestDetectorRoutesObjectiveAndVisionEvidence(t *testing.T) {
	tests := []struct {
		name, event, tag, category string
	}{
		{name: "objective", event: "objective", tag: "objective-contest", category: "objective-contest"},
		{name: "vision", event: "vision-loss", tag: "vision-uncertainty", category: "vision-uncertainty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var detector Detector
			frames := make([]model.LiveTelemetryFrame, 0, 13)
			for second := 0; second <= 12; second++ {
				probability := 0.1
				if second == 12 {
					probability = 0.9
				}
				frame := basicFrame(second, probability)
				frame.Events = []string{test.event, test.event}
				frames = append(frames, frame)
			}
			candidates := detector.Add(frames)
			if len(candidates) != 1 {
				t.Fatalf("got %d candidates, want 1", len(candidates))
			}
			candidate := candidates[0]
			if candidate.Category != test.category || !slices.Contains(candidate.Detection.ReasonTags, test.tag) {
				t.Fatalf("unexpected routed candidate: %+v", candidate)
			}
		})
	}
}

func TestVisionLossIsAValidNormalizedEvent(t *testing.T) {
	if !validateEvent("vision-loss") {
		t.Fatal("vision-loss should be accepted")
	}
	if validateEvent("ward-owner-name") {
		t.Fatal("unknown or identity-bearing event should be rejected")
	}
}

func reversalFrame(second int) model.LiveTelemetryFrame {
	probability := 0.75
	if second == 6 {
		probability = 0.2
	} else if second == 12 {
		probability = 0.85
	}
	return model.LiveTelemetryFrame{
		Second: second, WinProbability: probability, Events: []string{"damage", "kill"},
		Units: []model.LiveTelemetryUnit{
			{ID: "blue-carry", Team: "blue", Position: model.Point{X: 50, Y: 50}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
			{ID: "blue-support", Team: "blue", Position: model.Point{X: 90, Y: 50}, HP: 100, MaxHP: 100, Gold: 800, Alive: true},
			{ID: "red-top", Team: "red", Position: model.Point{X: 44, Y: 50}, HP: 100, MaxHP: 100, Gold: 900, Alive: true},
			{ID: "red-jungle", Team: "red", Position: model.Point{X: 56, Y: 50}, HP: 100, MaxHP: 100, Gold: 900, Alive: true},
		},
	}
}

func basicFrame(second int, probability float64) model.LiveTelemetryFrame {
	return model.LiveTelemetryFrame{
		Second: second, WinProbability: probability,
		Units: []model.LiveTelemetryUnit{
			{ID: "blue-carry", Team: "blue", Position: model.Point{X: 50, Y: 50}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
			{ID: "red-carry", Team: "red", Position: model.Point{X: 52, Y: 50}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
		},
	}
}
