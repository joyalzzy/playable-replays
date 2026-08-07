package telemetry

import (
	"fmt"
	"slices"
	"sort"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	maxTimelineFrames = 180
	maxTimelineEvents = 240
)

type anonymousUnit struct {
	trackID string
	side    string
}

type sampledTimelineFrame struct {
	sourceIndex int
	frame       model.TelemetryTimelineFrame
}

type sampledTimelineEvent struct {
	sourceIndex int
	event       model.TelemetryTimelineEvent
}

// timelineBuffer retains a deterministic, progressively downsampled visual
// trace. Raw IDs are used only as private map keys and are never serialized.
type timelineBuffer struct {
	unitAliases   map[string]anonymousUnit
	teamSides     map[string]string
	sideCounts    map[string]int
	framesSeen    int
	frameEvery    int
	frames        []sampledTimelineFrame
	latest        model.TelemetryTimelineFrame
	eventsSeen    int
	eventEvery    int
	events        []sampledTimelineEvent
	eventsDropped bool
}

func (b *timelineBuffer) Add(frames []model.LiveTelemetryFrame) {
	b.initialize()
	for _, frame := range frames {
		visual := b.anonymizeFrame(frame)
		sourceIndex := b.framesSeen
		b.framesSeen++
		b.latest = visual
		if sourceIndex%b.frameEvery == 0 {
			b.frames = append(b.frames, sampledTimelineFrame{sourceIndex: sourceIndex, frame: visual})
			b.compactFrames()
		}
		b.addEvents(frame)
	}
}

func (b *timelineBuffer) Snapshot(matchID string) model.TelemetryTimeline {
	b.initialize()
	frames := make([]model.TelemetryTimelineFrame, 0, min(maxTimelineFrames, len(b.frames)+1))
	for _, sample := range b.frames {
		frames = append(frames, cloneTimelineFrame(sample.frame))
	}
	if b.framesSeen > 0 && (len(frames) == 0 || frames[len(frames)-1].Second != b.latest.Second) {
		frames = append(frames, cloneTimelineFrame(b.latest))
	}
	events := make([]model.TelemetryTimelineEvent, len(b.events))
	for index, sample := range b.events {
		events[index] = sample.event
	}
	return model.TelemetryTimeline{
		MatchID: matchID, SourceFrameCount: b.framesSeen, SampleEvery: b.frameEvery,
		Truncated: b.framesSeen > len(frames) || b.eventsDropped,
		Frames:    frames, Events: events,
	}
}

// AnonymizeDetection replaces source unit IDs with the same stable A/B track
// aliases used by the public timeline. Detector scores, signals, labels, and
// timestamps are preserved.
func (b *timelineBuffer) AnonymizeDetection(detection model.TelemetryDetection) model.TelemetryDetection {
	b.initialize()
	detection.ReasonTags = slices.Clone(detection.ReasonTags)
	detection.SemanticEvidence.OneVersusManyUnitIDs = b.anonymizeIDs(detection.SemanticEvidence.OneVersusManyUnitIDs)
	detection.SemanticEvidence.SuccessfulEscapeUnitIDs = b.anonymizeIDs(detection.SemanticEvidence.SuccessfulEscapeUnitIDs)
	if detection.SemanticEvidence.TeamFightReversalSecond != nil {
		second := *detection.SemanticEvidence.TeamFightReversalSecond
		detection.SemanticEvidence.TeamFightReversalSecond = &second
	}
	return detection
}

func (b *timelineBuffer) anonymizeIDs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if alias, ok := b.unitAliases[value]; ok {
			result = append(result, alias.trackID)
		}
	}
	return result
}

func (b *timelineBuffer) initialize() {
	if b.unitAliases == nil {
		b.unitAliases = map[string]anonymousUnit{}
		b.teamSides = map[string]string{}
		b.sideCounts = map[string]int{"a": 0, "b": 0}
	}
	if b.frameEvery == 0 {
		b.frameEvery = 1
	}
	if b.eventEvery == 0 {
		b.eventEvery = 1
	}
}

func (b *timelineBuffer) anonymizeFrame(frame model.LiveTelemetryFrame) model.TelemetryTimelineFrame {
	units := make([]model.TelemetryTimelineUnit, 0, len(frame.Units))
	for _, unit := range frame.Units {
		alias, ok := b.unitAliases[unit.ID]
		if !ok {
			side, found := b.teamSides[unit.Team]
			if !found {
				side = "a"
				if len(b.teamSides) > 0 {
					side = "b"
				}
				b.teamSides[unit.Team] = side
			}
			b.sideCounts[side]++
			prefix := "A"
			if side == "b" {
				prefix = "B"
			}
			alias = anonymousUnit{trackID: fmt.Sprintf("%s%d", prefix, b.sideCounts[side]), side: side}
			b.unitAliases[unit.ID] = alias
		}
		units = append(units, model.TelemetryTimelineUnit{
			TrackID: alias.trackID, Side: alias.side, Position: unit.Position, Alive: unit.Alive,
		})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].TrackID < units[j].TrackID })
	return model.TelemetryTimelineFrame{Second: frame.Second, Units: units}
}

func (b *timelineBuffer) addEvents(frame model.LiveTelemetryFrame) {
	counts := map[string]int{}
	for _, eventType := range frame.Events {
		counts[eventType]++
	}
	types := make([]string, 0, len(counts))
	for eventType := range counts {
		types = append(types, eventType)
	}
	sort.Strings(types)
	for _, eventType := range types {
		sourceIndex := b.eventsSeen
		b.eventsSeen++
		if sourceIndex%b.eventEvery == 0 {
			b.events = append(b.events, sampledTimelineEvent{
				sourceIndex: sourceIndex,
				event:       model.TelemetryTimelineEvent{Second: frame.Second, Type: eventType, Count: counts[eventType]},
			})
			b.compactEvents()
		} else {
			b.eventsDropped = true
		}
	}
}

func (b *timelineBuffer) compactFrames() {
	for len(b.frames) > maxTimelineFrames-1 {
		b.frameEvery *= 2
		b.frames = filterFrames(b.frames, b.frameEvery)
	}
}

func (b *timelineBuffer) compactEvents() {
	for len(b.events) > maxTimelineEvents {
		b.eventEvery *= 2
		b.events = filterEvents(b.events, b.eventEvery)
		b.eventsDropped = true
	}
}

func filterFrames(samples []sampledTimelineFrame, every int) []sampledTimelineFrame {
	kept := make([]sampledTimelineFrame, 0, len(samples)/2+1)
	for _, sample := range samples {
		if sample.sourceIndex%every == 0 {
			kept = append(kept, sample)
		}
	}
	return kept
}

func filterEvents(samples []sampledTimelineEvent, every int) []sampledTimelineEvent {
	kept := make([]sampledTimelineEvent, 0, len(samples)/2+1)
	for _, sample := range samples {
		if sample.sourceIndex%every == 0 {
			kept = append(kept, sample)
		}
	}
	return kept
}

func cloneTimelineFrame(frame model.TelemetryTimelineFrame) model.TelemetryTimelineFrame {
	return model.TelemetryTimelineFrame{Second: frame.Second, Units: slices.Clone(frame.Units)}
}
