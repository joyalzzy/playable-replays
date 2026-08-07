package telemetry

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestTimelineIsBoundedAnonymousAndKeepsLatestFrame(t *testing.T) {
	var buffer timelineBuffer
	frames := make([]model.LiveTelemetryFrame, 0, 500)
	for second := 0; second < 500; second++ {
		units := []model.LiveTelemetryUnit{
			{ID: "publisher-player-alice", Team: "publisher-blue-team", Position: model.Point{X: float64(second % 100), Y: 20}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
			{ID: "publisher-player-bob", Team: "publisher-red-team", Position: model.Point{X: float64(100 - second%100), Y: 80}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
		}
		if second%2 == 1 {
			units[0], units[1] = units[1], units[0]
		}
		frames = append(frames, model.LiveTelemetryFrame{Second: second, WinProbability: 0.5, Events: []string{"damage"}, Units: units})
	}
	buffer.Add(frames)
	timeline := buffer.Snapshot("telemetry-match-1")

	if len(timeline.Frames) > maxTimelineFrames || len(timeline.Events) > maxTimelineEvents {
		t.Fatalf("timeline exceeded bounds: %d frames, %d events", len(timeline.Frames), len(timeline.Events))
	}
	if timeline.SourceFrameCount != 500 || timeline.SampleEvery <= 1 || !timeline.Truncated {
		t.Fatalf("unexpected sampling metadata: %+v", timeline)
	}
	latest := timeline.Frames[len(timeline.Frames)-1]
	if latest.Second != 499 || latest.Units[0].TrackID != "A1" || latest.Units[1].TrackID != "B1" {
		t.Fatalf("latest frame or stable aliases missing: %+v", latest)
	}
	encoded, err := json.Marshal(timeline)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"alice", "bob", "publisher-blue-team", "publisher-red-team", "gold", "maxHp"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("timeline leaked %q: %s", private, encoded)
		}
	}
}

func TestTimelineAggregatesNormalizedEvents(t *testing.T) {
	var buffer timelineBuffer
	buffer.Add([]model.LiveTelemetryFrame{{
		Second: 3, WinProbability: 0.5, Events: []string{"kill", "damage", "damage"},
		Units: []model.LiveTelemetryUnit{
			{ID: "blue", Team: "blue", Position: model.Point{X: 10, Y: 20}, HP: 100, MaxHP: 100, Alive: true},
			{ID: "red", Team: "red", Position: model.Point{X: 90, Y: 80}, HP: 100, MaxHP: 100, Alive: true},
		},
	}})
	timeline := buffer.Snapshot("telemetry-match-1")
	if len(timeline.Events) != 2 || timeline.Events[0].Type != "damage" || timeline.Events[0].Count != 2 || timeline.Events[1].Type != "kill" {
		t.Fatalf("events were not deterministically aggregated: %+v", timeline.Events)
	}
}

func TestDetectionEvidenceUsesTimelineAliases(t *testing.T) {
	var buffer timelineBuffer
	buffer.Add([]model.LiveTelemetryFrame{{
		Second: 0,
		Units: []model.LiveTelemetryUnit{
			{ID: "private-source-one", Team: "source-team-one", Position: model.Point{X: 10, Y: 20}, Alive: true},
			{ID: "private-source-two", Team: "source-team-two", Position: model.Point{X: 90, Y: 80}, Alive: true},
		},
	}})
	second := 5
	result := buffer.AnonymizeDetection(model.TelemetryDetection{
		ReasonTags: []string{"successful-escape"},
		SemanticEvidence: model.TelemetrySemanticEvidence{
			OneVersusManyUnitIDs: []string{"private-source-one"}, SuccessfulEscapeUnitIDs: []string{"private-source-two"},
			TeamFightReversalSecond: &second,
		},
	})
	if len(result.SemanticEvidence.OneVersusManyUnitIDs) != 1 || result.SemanticEvidence.OneVersusManyUnitIDs[0] != "A1" ||
		len(result.SemanticEvidence.SuccessfulEscapeUnitIDs) != 1 || result.SemanticEvidence.SuccessfulEscapeUnitIDs[0] != "B1" {
		t.Fatalf("source evidence was not anonymized: %+v", result.SemanticEvidence)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-source") || result.SemanticEvidence.TeamFightReversalSecond == &second {
		t.Fatalf("anonymized evidence retained source identity or pointer alias: %s", encoded)
	}
}
