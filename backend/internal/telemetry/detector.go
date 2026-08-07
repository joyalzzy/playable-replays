package telemetry

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/drafts"
	"github.com/joyalzzy/playable-replays/backend/internal/highlight"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	windowSeconds          = 12
	strideSeconds          = 2
	detectionThreshold     = 0.65
	maxOverlapFraction     = 0.5
	eventRateCap           = 2.0
	engagementRadius       = 20.0
	minimumExposureSeconds = 2
	escapeSafeRadius       = 35.0
	lowHealthFraction      = 0.35
	minimumReversalSwing   = 0.25
	minimumCombatEvents    = 2
	mapDiagonal            = 141.4213562373095
)

type qualifiedWindow struct {
	rawScore  float64
	detection model.TelemetryDetection
}

// Detector incrementally evaluates fully covered windows while preserving the
// same score, tags, evidence, and overlap suppression as ml.telemetry.
type Detector struct {
	frames           []model.LiveTelemetryFrame
	nextWindowSecond int
	initialized      bool
	qualified        []qualifiedWindow
}

func (d *Detector) Add(frames []model.LiveTelemetryFrame) []model.TelemetryCandidate {
	d.frames = append(d.frames, cloneFrames(frames)...)
	if !d.initialized && len(d.frames) > 0 {
		d.nextWindowSecond = d.frames[0].Second
		d.initialized = true
	}
	d.evaluateCoveredWindows()
	return d.Candidates("provisional", nil)
}

func (d *Detector) Candidates(status string, draftStatuses map[string]string) []model.TelemetryCandidate {
	selected := slices.Clone(d.qualified)
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].rawScore == selected[j].rawScore {
			return selected[i].detection.StartSecond < selected[j].detection.StartSecond
		}
		return selected[i].rawScore > selected[j].rawScore
	})
	kept := make([]qualifiedWindow, 0, len(selected))
	for _, candidate := range selected {
		if overlapsSelected(candidate.detection, kept) {
			continue
		}
		kept = append(kept, candidate)
	}
	result := make([]model.TelemetryCandidate, 0, len(kept))
	for _, candidate := range kept {
		id := candidateID(candidate.detection)
		draftStatus := draftStatuses[id]
		if draftStatus == "" {
			draftStatus = "not-created"
		}
		result = append(result, model.TelemetryCandidate{
			ID: id, Status: status, Category: drafts.CategoryFor(candidate.detection.ReasonTags),
			DraftStatus: draftStatus, Detection: cloneDetection(candidate.detection),
		})
	}
	return result
}

func (d *Detector) evaluateCoveredWindows() {
	for {
		startIndex := -1
		for index, frame := range d.frames {
			if frame.Second >= d.nextWindowSecond {
				startIndex = index
				break
			}
		}
		if startIndex < 0 {
			return
		}
		startSecond := d.frames[startIndex].Second
		endIndex := -1
		for index := startIndex + 1; index < len(d.frames); index++ {
			if d.frames[index].Second >= startSecond+windowSeconds {
				endIndex = index
				break
			}
		}
		if endIndex < 0 {
			return
		}
		window := d.frames[startIndex : endIndex+1]
		signals := extractSignals(window)
		rawScore := highlight.Score(signals)
		if rawScore >= detectionThreshold {
			detection := model.TelemetryDetection{
				SchemaVersion: "1.0", StartSecond: startSecond, EndSecond: d.frames[endIndex].Second,
				Score: highlight.RoundedScore(signals), ReasonTags: reasonTags(signals), Signals: signals,
				SemanticEvidence: semanticEvidence(window),
			}
			addSemanticTags(&detection, window)
			d.qualified = append(d.qualified, qualifiedWindow{rawScore: rawScore, detection: detection})
		}
		d.nextWindowSecond = startSecond + strideSeconds
		d.trimEvaluatedFrames()
	}
}

func (d *Detector) trimEvaluatedFrames() {
	remove := 0
	for remove < len(d.frames) && d.frames[remove].Second < d.nextWindowSecond {
		remove++
	}
	if remove > 0 {
		d.frames = slices.Clone(d.frames[remove:])
	}
}

func extractSignals(frames []model.LiveTelemetryFrame) model.Signals {
	probabilityMin, probabilityMax := frames[0].WinProbability, frames[0].WinProbability
	eventCount := 0
	nearest := mapDiagonal
	resourceAsymmetry := 0.0
	for _, frame := range frames {
		probabilityMin = min(probabilityMin, frame.WinProbability)
		probabilityMax = max(probabilityMax, frame.WinProbability)
		eventCount += len(frame.Events)
		nearest = min(nearest, nearestOpponentDistance(frame))
		resourceAsymmetry = max(resourceAsymmetry, frameResourceAsymmetry(frame))
	}
	duration := frames[len(frames)-1].Second - frames[0].Second
	return model.Signals{
		WinProbabilitySwing: probabilityMax - probabilityMin,
		EventDensity:        min(1, float64(eventCount)/float64(duration)/eventRateCap),
		EntityProximity:     nearest / mapDiagonal,
		ResourceAsymmetry:   resourceAsymmetry,
	}
}

func reasonTags(signals model.Signals) []string {
	tags := []string{}
	if signals.WinProbabilitySwing >= 0.65 {
		tags = append(tags, "win-probability-swing")
	}
	if signals.EventDensity >= 0.75 {
		tags = append(tags, "high-event-density")
	}
	if signals.EntityProximity <= 0.25 {
		tags = append(tags, "team-fight")
	}
	if signals.ResourceAsymmetry >= 0.6 {
		tags = append(tags, "resource-disadvantage")
	}
	if len(tags) == 0 {
		return []string{"strategic-decision"}
	}
	return tags
}

func semanticEvidence(frames []model.LiveTelemetryFrame) model.TelemetrySemanticEvidence {
	return model.TelemetrySemanticEvidence{
		OneVersusManyUnitIDs:    oneVersusMany(frames),
		SuccessfulEscapeUnitIDs: successfulEscapes(frames),
		TeamFightReversalSecond: teamFightReversal(frames),
	}
}

func oneVersusMany(frames []model.LiveTelemetryFrame) []string {
	starts := map[string]int{}
	previous := map[string]bool{}
	detected := map[string]bool{}
	for _, frame := range frames {
		qualifying := map[string]bool{}
		for _, unit := range frame.Units {
			if unit.Alive && isOneVersusMany(unit, frame.Units) {
				qualifying[unit.ID] = true
			}
		}
		for id := range qualifying {
			if !previous[id] {
				starts[id] = frame.Second
			}
			if frame.Second-starts[id] >= minimumExposureSeconds {
				detected[id] = true
			}
		}
		for id := range previous {
			if !qualifying[id] {
				delete(starts, id)
			}
		}
		previous = qualifying
	}
	return sortedKeys(detected)
}

func successfulEscapes(frames []model.LiveTelemetryFrame) []string {
	exposed := map[string]bool{}
	safeStarts := map[string]int{}
	detected := map[string]bool{}
	for _, frame := range frames {
		for _, unit := range frame.Units {
			if !unit.Alive {
				delete(exposed, unit.ID)
				delete(safeStarts, unit.ID)
				delete(detected, unit.ID)
				continue
			}
			nearest, found := nearestDistanceToOpponent(unit, frame.Units)
			if found && unit.HP/unit.MaxHP <= lowHealthFraction && nearest <= engagementRadius {
				exposed[unit.ID] = true
			}
			safe := exposed[unit.ID] && found && nearest >= escapeSafeRadius
			if safe {
				if _, ok := safeStarts[unit.ID]; !ok {
					safeStarts[unit.ID] = frame.Second
				}
				if frame.Second-safeStarts[unit.ID] >= minimumExposureSeconds {
					detected[unit.ID] = true
				}
			} else {
				delete(safeStarts, unit.ID)
				delete(detected, unit.ID)
			}
		}
	}
	return sortedKeys(detected)
}

func teamFightReversal(frames []model.LiveTelemetryFrame) *int {
	combatEvents := 0
	for _, frame := range frames {
		for _, event := range frame.Events {
			if event == "damage" || event == "kill" {
				combatEvents++
			}
		}
	}
	if combatEvents < minimumCombatEvents || len(frames) < 3 {
		return nil
	}
	finalProbability := frames[len(frames)-1].WinProbability
	bestMagnitude := -1.0
	bestSecond := 0
	for index := 1; index < len(frames)-1; index++ {
		turning := frames[index].WinProbability
		priorMin, priorMax := frames[0].WinProbability, frames[0].WinProbability
		for prior := 0; prior < index; prior++ {
			priorMin = min(priorMin, frames[prior].WinProbability)
			priorMax = max(priorMax, frames[prior].WinProbability)
		}
		recovery := min(priorMax-turning, finalProbability-turning)
		collapse := min(turning-priorMin, turning-finalProbability)
		magnitude := max(recovery, collapse)
		if magnitude >= minimumReversalSwing && nearestOpponentDistance(frames[index]) <= engagementRadius &&
			(magnitude > bestMagnitude || magnitude == bestMagnitude && frames[index].Second < bestSecond) {
			bestMagnitude, bestSecond = magnitude, frames[index].Second
		}
	}
	if bestMagnitude < 0 {
		return nil
	}
	return &bestSecond
}

func addSemanticTags(detection *model.TelemetryDetection, frames []model.LiveTelemetryFrame) {
	if len(detection.SemanticEvidence.OneVersusManyUnitIDs) > 0 {
		detection.ReasonTags = append(detection.ReasonTags, "one-versus-many")
	}
	if len(detection.SemanticEvidence.SuccessfulEscapeUnitIDs) > 0 {
		detection.ReasonTags = append(detection.ReasonTags, "successful-escape")
	}
	if detection.SemanticEvidence.TeamFightReversalSecond != nil {
		detection.ReasonTags = append(detection.ReasonTags, "team-fight-reversal")
	}
	if windowContainsEvent(frames, "objective") {
		detection.ReasonTags = append(detection.ReasonTags, "objective-contest")
	}
	if windowContainsEvent(frames, "vision-loss") {
		detection.ReasonTags = append(detection.ReasonTags, "vision-uncertainty")
	}
}

func windowContainsEvent(frames []model.LiveTelemetryFrame, wanted string) bool {
	for _, frame := range frames {
		if slices.Contains(frame.Events, wanted) {
			return true
		}
	}
	return false
}

func nearestOpponentDistance(frame model.LiveTelemetryFrame) float64 {
	nearest := mapDiagonal
	for index, unit := range frame.Units {
		if !unit.Alive {
			continue
		}
		for _, opponent := range frame.Units[index+1:] {
			if opponent.Alive && opponent.Team != unit.Team {
				nearest = min(nearest, distance(unit, opponent))
			}
		}
	}
	return nearest
}

func nearestDistanceToOpponent(unit model.LiveTelemetryUnit, units []model.LiveTelemetryUnit) (float64, bool) {
	nearest := mapDiagonal
	found := false
	for _, other := range units {
		if other.Alive && other.Team != unit.Team {
			nearest, found = min(nearest, distance(unit, other)), true
		}
	}
	return nearest, found
}

func frameResourceAsymmetry(frame model.LiveTelemetryFrame) float64 {
	type totals struct{ hp, maxHP, gold float64 }
	byTeam := map[string]totals{}
	for _, unit := range frame.Units {
		value := byTeam[unit.Team]
		value.hp, value.maxHP, value.gold = value.hp+unit.HP, value.maxHP+unit.MaxHP, value.gold+unit.Gold
		byTeam[unit.Team] = value
	}
	values := make([]totals, 0, 2)
	for _, value := range byTeam {
		values = append(values, value)
	}
	first, second := values[0], values[1]
	hpGap := math.Abs(first.hp/first.maxHP - second.hp/second.maxHP)
	goldTotal := first.gold + second.gold
	goldGap := 0.0
	if goldTotal > 0 {
		goldGap = math.Abs(first.gold-second.gold) / goldTotal
	}
	return (hpGap + goldGap) / 2
}

func isOneVersusMany(unit model.LiveTelemetryUnit, units []model.LiveTelemetryUnit) bool {
	allies, opponents := 0, 0
	for _, other := range units {
		if !other.Alive || other.ID == unit.ID || distance(unit, other) > engagementRadius {
			continue
		}
		if other.Team == unit.Team {
			allies++
		} else {
			opponents++
		}
	}
	return allies == 0 && opponents >= 2
}

func distance(first, second model.LiveTelemetryUnit) float64 {
	return math.Hypot(first.Position.X-second.Position.X, first.Position.Y-second.Position.Y)
}

func overlapsSelected(detection model.TelemetryDetection, selected []qualifiedWindow) bool {
	for _, existing := range selected {
		overlap := max(0, min(detection.EndSecond, existing.detection.EndSecond)-max(detection.StartSecond, existing.detection.StartSecond))
		shorter := min(detection.EndSecond-detection.StartSecond, existing.detection.EndSecond-existing.detection.StartSecond)
		if shorter > 0 && float64(overlap)/float64(shorter) > maxOverlapFraction {
			return true
		}
	}
	return false
}

func candidateID(detection model.TelemetryDetection) string {
	return fmt.Sprintf("candidate-%d-%d", detection.StartSecond, detection.EndSecond)
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func cloneFrames(frames []model.LiveTelemetryFrame) []model.LiveTelemetryFrame {
	cloned := make([]model.LiveTelemetryFrame, len(frames))
	for index, frame := range frames {
		frame.Events = slices.Clone(frame.Events)
		frame.Units = slices.Clone(frame.Units)
		cloned[index] = frame
	}
	return cloned
}

func cloneDetection(detection model.TelemetryDetection) model.TelemetryDetection {
	detection.ReasonTags = slices.Clone(detection.ReasonTags)
	detection.SemanticEvidence.OneVersusManyUnitIDs = slices.Clone(detection.SemanticEvidence.OneVersusManyUnitIDs)
	detection.SemanticEvidence.SuccessfulEscapeUnitIDs = slices.Clone(detection.SemanticEvidence.SuccessfulEscapeUnitIDs)
	if detection.SemanticEvidence.TeamFightReversalSecond != nil {
		second := *detection.SemanticEvidence.TeamFightReversalSecond
		detection.SemanticEvidence.TeamFightReversalSecond = &second
	}
	return detection
}

func validateEvent(event string) bool {
	return event == "damage" || event == "kill" || event == "objective" || event == "vision-loss"
}

func validateSource(source string) bool {
	return source == "synthetic" || source == "authorized"
}

func normalizeSource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}
