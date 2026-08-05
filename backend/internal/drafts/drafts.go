package drafts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/highlight"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	BundleVersion = "2.1"
	DraftStatus   = "draft"
	maxRecordSize = 1 << 20
)

type Bundle struct {
	Version string  `json:"version"`
	Drafts  []Draft `json:"drafts"`
}

type Draft struct {
	Status   string       `json:"status"`
	Scenario model.Moment `json:"scenario"`
}

// FieldIssue lets the analyst workbench place a validation message beside the
// part of the scenario that needs attention instead of showing one opaque
// publish failure.
type FieldIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// FromNDJSON converts every non-empty detector record into an intentionally
// incomplete version 2.1 scenario draft.
func FromNDJSON(reader io.Reader) (Bundle, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxRecordSize)
	bundle := Bundle{Version: BundleVersion, Drafts: []Draft{}}
	line := 0
	for scanner.Scan() {
		line++
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		var detection model.TelemetryDetection
		if err := decodeStrict(data, &detection); err != nil {
			return Bundle{}, fmt.Errorf("NDJSON line %d: %w", line, err)
		}
		if err := validateDetection(detection); err != nil {
			return Bundle{}, fmt.Errorf("NDJSON line %d: %w", line, err)
		}
		bundle.Drafts = append(bundle.Drafts, Draft{
			Status:   DraftStatus,
			Scenario: starterScenario(detection),
		})
	}
	if err := scanner.Err(); err != nil {
		return Bundle{}, fmt.Errorf("read detector NDJSON: %w", err)
	}
	if len(bundle.Drafts) == 0 {
		return Bundle{}, fmt.Errorf("detector NDJSON contains no records")
	}
	return bundle, nil
}

func ReadBundle(reader io.Reader) (Bundle, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 16<<20))
	if err != nil {
		return Bundle{}, fmt.Errorf("read draft bundle: %w", err)
	}
	var bundle Bundle
	if err := decodeStrict(data, &bundle); err != nil {
		return Bundle{}, fmt.Errorf("decode draft bundle: %w", err)
	}
	if bundle.Version != BundleVersion {
		return Bundle{}, fmt.Errorf("unsupported draft fixture version %q", bundle.Version)
	}
	if len(bundle.Drafts) == 0 {
		return Bundle{}, fmt.Errorf("draft bundle contains no scenarios")
	}
	for index, draft := range bundle.Drafts {
		if draft.Status != DraftStatus {
			return Bundle{}, fmt.Errorf("draft %d has unsupported status %q", index, draft.Status)
		}
	}
	return bundle, nil
}

func CategoryFor(reasonTags []string) string {
	tags := make([]string, 0, len(reasonTags))
	for _, tag := range reasonTags {
		tags = append(tags, strings.ToLower(strings.TrimSpace(tag)))
	}
	if slices.Contains(tags, "team-fight-reversal") {
		return "team-fight-engagement"
	}
	if slices.Contains(tags, "successful-escape") {
		return "escape"
	}
	if containsFragment(tags, "objective") {
		return "objective-contest"
	}
	if containsFragment(tags, "vision") || containsFragment(tags, "fog") || containsFragment(tags, "uncertainty") {
		return "vision-uncertainty"
	}
	if containsFragment(tags, "resource") || containsFragment(tags, "gold") || containsFragment(tags, "trade") {
		return "resource-trade"
	}
	if slices.Contains(tags, "one-versus-many") {
		return "positioning"
	}
	if slices.Contains(tags, "team-fight") {
		return "team-fight-engagement"
	}
	return "positioning"
}

// CompletionIssues reports why a draft cannot enter a published fixture pack.
func CompletionIssues(draft Draft) []string {
	fieldIssues := CompletionFieldIssues(draft)
	issues := make([]string, 0, len(fieldIssues))
	for _, issue := range fieldIssues {
		issues = append(issues, issue.Message)
	}
	return issues
}

// CompletionFieldIssues reports the same publication gate as
// CompletionIssues, with stable field names for the local authoring UI.
func CompletionFieldIssues(draft Draft) []FieldIssue {
	issues := []string{}
	moment := draft.Scenario
	if draft.Status != DraftStatus {
		issues = append(issues, "status must remain draft until publication")
	}
	if moment.SourceDetection == nil {
		issues = append(issues, "sourceDetection is required")
	}
	if strings.TrimSpace(moment.Title) == "" {
		issues = append(issues, "scenario title is required")
	}
	if strings.TrimSpace(moment.Description) == "" {
		issues = append(issues, "scenario description is required")
	}
	if strings.TrimSpace(moment.Map) == "" {
		issues = append(issues, "scenario map is required")
	}
	if moment.MaxTurns < 1 {
		issues = append(issues, "maxTurns must be authored")
	}
	if strings.TrimSpace(moment.ControlledUnitID) == "" || len(moment.Units) < 2 {
		issues = append(issues, "controlled unit and at least two synthetic units are required")
	}
	if strings.TrimSpace(moment.Rules.Victory.Kind) == "" {
		issues = append(issues, "victory, defeat, and simulator rules are required")
	}
	if strings.TrimSpace(moment.Authoring.SkillLevel) == "" {
		issues = append(issues, "analyst must assign a skill level")
	}
	if strings.TrimSpace(moment.Authoring.AnalystRationale) == "" {
		issues = append(issues, "analyst rationale is incomplete")
	}
	if len(moment.Authoring.IntendedTradeoffs) < 2 {
		issues = append(issues, "at least two intended tradeoffs are incomplete")
	}
	if len(moment.Authoring.PlausibleAlternatives) < 2 {
		issues = append(issues, "at least two plausible alternatives are incomplete")
	}
	if len(moment.Authoring.AcceptanceTests) < 2 {
		issues = append(issues, "win and loss acceptance tests are incomplete")
	}
	if len(issues) == 0 {
		if err := fixtures.ValidateMoment(moment); err != nil {
			issues = append(issues, err.Error())
		} else if err := fixtures.ValidateAcceptance([]model.Moment{moment}); err != nil {
			issues = append(issues, err.Error())
		}
	}
	result := make([]FieldIssue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, FieldIssue{Field: fieldForIssue(issue), Message: issue})
	}
	return result
}

func fieldForIssue(issue string) string {
	lower := strings.ToLower(issue)
	switch {
	case strings.Contains(lower, "status"):
		return "status"
	case strings.Contains(lower, "sourcedetection"), strings.Contains(lower, "source detection"),
		strings.Contains(lower, "source window"), strings.Contains(lower, "source reason"),
		strings.Contains(lower, "signals"), strings.Contains(lower, "evidence"),
		strings.Contains(lower, "start time"):
		return "provenance"
	case strings.Contains(lower, "title"):
		return "title"
	case strings.Contains(lower, "description") && !strings.Contains(lower, "victory"):
		return "description"
	case strings.Contains(lower, "map") && !strings.Contains(lower, "inside the map"):
		return "map"
	case strings.Contains(lower, "skill level"):
		return "difficulty"
	case strings.Contains(lower, "rationale"):
		return "rationale"
	case strings.Contains(lower, "tradeoff"):
		return "tradeoffs"
	case strings.Contains(lower, "alternative"):
		return "alternatives"
	case strings.Contains(lower, "acceptance"), strings.Contains(lower, "fixture acceptance"):
		return "acceptanceTests"
	case strings.Contains(lower, "unit"), strings.Contains(lower, "both teams"), strings.Contains(lower, "combat state"):
		return "units"
	case strings.Contains(lower, "terrain"), strings.Contains(lower, "river core"), strings.Contains(lower, "canonical position"), strings.Contains(lower, "mechanic briefing"):
		return "terrain"
	default:
		return "rules"
	}
}

func ValidateDraft(draft Draft) error {
	issues := CompletionIssues(draft)
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("draft is not publishable:\n- %s", strings.Join(issues, "\n- "))
}

// PreparePack refuses incomplete drafts, duplicate identities, invalid pack
// coverage, or acceptance-test failures.
func PreparePack(draft Draft, base []model.Moment) ([]model.Moment, error) {
	if err := ValidateDraft(draft); err != nil {
		return nil, err
	}
	for _, moment := range base {
		if moment.ID == draft.Scenario.ID {
			return nil, fmt.Errorf("base pack already contains moment id %q", moment.ID)
		}
		if moment.Slug == draft.Scenario.Slug {
			return nil, fmt.Errorf("base pack already contains moment slug %q", moment.Slug)
		}
	}
	moments := append(slices.Clone(base), draft.Scenario)
	if err := fixtures.ValidateLibrary(moments); err != nil {
		return nil, err
	}
	if err := fixtures.ValidateAcceptance(moments); err != nil {
		return nil, err
	}
	return moments, nil
}

func WriteBundle(path string, bundle Bundle, force bool) error {
	return writeJSON(path, bundle, force)
}

func WritePack(path string, moments []model.Moment, force bool) error {
	return writeJSON(path, struct {
		Version string         `json:"version"`
		Moments []model.Moment `json:"moments"`
	}{Version: BundleVersion, Moments: moments}, force)
}

func starterScenario(detection model.TelemetryDetection) model.Moment {
	category := CategoryFor(detection.ReasonTags)
	identity := fmt.Sprintf("%s-%d-%d", category, detection.StartSecond, detection.EndSecond)
	source := cloneDetection(detection)
	return model.Moment{
		ID:               identity,
		Slug:             identity,
		StartTimeSeconds: detection.StartSecond,
		Seed:             deterministicSeed(detection),
		ReasonTags:       slices.Clone(detection.ReasonTags),
		Signals:          detection.Signals,
		SourceDetection:  &source,
		Units:            []model.Unit{},
		Rules: model.ScenarioRules{
			Terrain:          []model.TerrainFeature{},
			ReferencePlan:    []model.Action{},
			ReferenceReasons: []string{},
			ReferenceContinuations: map[string][]model.Action{
				"move": {}, "hold": {}, "contest": {}, "retreat": {},
			},
			ActionDefaults: map[string]model.Action{},
		},
		Authoring: model.ScenarioAuthoring{
			Category:              category,
			IntendedTradeoffs:     []string{},
			PlausibleAlternatives: []model.ScenarioAlternative{},
			AcceptanceTests:       []model.ScenarioAcceptanceTest{},
		},
	}
}

func validateDetection(detection model.TelemetryDetection) error {
	if detection.SchemaVersion != "1.0" {
		return fmt.Errorf("unsupported detector schema version %q", detection.SchemaVersion)
	}
	if detection.StartSecond < 0 || detection.EndSecond <= detection.StartSecond {
		return fmt.Errorf("detector window must have increasing non-negative timestamps")
	}
	if !finiteUnit(detection.Score) || len(detection.ReasonTags) == 0 || !normalizedSignals(detection.Signals) {
		return fmt.Errorf("detector score, reason tags, or signals are invalid")
	}
	seenTags := map[string]bool{}
	for _, tag := range detection.ReasonTags {
		if strings.TrimSpace(tag) == "" || seenTags[tag] {
			return fmt.Errorf("reason tags must be non-empty and unique")
		}
		seenTags[tag] = true
	}
	if detection.Score != highlight.RoundedScore(detection.Signals) {
		return fmt.Errorf("detector score %.4f does not match canonical signals %.4f", detection.Score, highlight.RoundedScore(detection.Signals))
	}
	if err := validateEvidence(detection.SemanticEvidence.OneVersusManyUnitIDs); err != nil {
		return fmt.Errorf("one-versus-many evidence: %w", err)
	}
	if err := validateEvidence(detection.SemanticEvidence.SuccessfulEscapeUnitIDs); err != nil {
		return fmt.Errorf("successful-escape evidence: %w", err)
	}
	if second := detection.SemanticEvidence.TeamFightReversalSecond; second != nil && (*second < detection.StartSecond || *second > detection.EndSecond) {
		return fmt.Errorf("team-fight reversal timestamp is outside the detector window")
	}
	return nil
}

func validateEvidence(ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			return fmt.Errorf("unit IDs must be non-empty and unique")
		}
		seen[id] = true
	}
	return nil
}

func normalizedSignals(signals model.Signals) bool {
	return finiteUnit(signals.WinProbabilitySwing) && finiteUnit(signals.EventDensity) &&
		finiteUnit(signals.EntityProximity) && finiteUnit(signals.ResourceAsymmetry)
}

func finiteUnit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func containsFragment(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
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

func deterministicSeed(detection model.TelemetryDetection) int64 {
	hash := fnv.New64a()
	fmt.Fprintf(hash, "%s:%d:%d:%s", detection.SchemaVersion, detection.StartSecond, detection.EndSecond, strings.Join(detection.ReasonTags, ","))
	seed := int64(hash.Sum64())
	if seed == 0 {
		return 1
	}
	return seed
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON object")
	}
	return nil
}

func writeJSON(path string, value any, force bool) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	data = append(data, '\n')
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite %s without --force", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect output %s: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
