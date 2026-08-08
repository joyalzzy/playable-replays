package fixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	fixtureVersion    = "3.0"
	minimumPackSize   = 1
	maximumPackSize   = 3
	minimumTradeoffs  = 2
	minimumAlternates = 2
	minimumTests      = 2
	maxUnitsPerMoment = 64
)

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	categories  = []string{
		"objective-contest",
		"team-fight-engagement",
		"escape",
		"positioning",
		"resource-trade",
		"vision-uncertainty",
	}
	skillLevels               = []string{"beginner", "intermediate", "advanced"}
	actionTypes               = []string{"move", "hold", "contest", "retreat"}
	canonicalTerrainLandmarks = map[string]model.Point{
		"base-gate":   {X: 16, Y: 82},
		"tower-zone":  {X: 30, Y: 69},
		"lane-pocket": {X: 20, Y: 78},
		"exit-pocket": {X: 11, Y: 63},
		"exit-zone":   {X: 12, Y: 78},
		"river":       {X: 50, Y: 52},
	}
	canonicalObjectiveLandmarks = map[string]model.Point{}
	scenarioSpecificMechanicIDs = map[string]bool{
		"baron-pit": true,
	}
)

type fileFormat struct {
	Version string         `json:"version"`
	Moments []model.Moment `json:"moments"`
}

func Load(path string) ([]model.Moment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixtures: %w", err)
	}
	var file fileFormat
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("decode fixtures: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode fixtures: expected one JSON document")
	}
	if file.Version != fixtureVersion {
		return nil, fmt.Errorf("unsupported fixture version %q", file.Version)
	}
	if err := ValidateLibrary(file.Moments); err != nil {
		return nil, err
	}
	return file.Moments, nil
}

// ValidateLibrary applies the complete publishable-pack contract without
// reading or writing a file.
func ValidateLibrary(moments []model.Moment) error {
	if len(moments) < minimumPackSize || len(moments) > maximumPackSize {
		return fmt.Errorf("fixture pack must contain %d to %d moments, got %d", minimumPackSize, maximumPackSize, len(moments))
	}
	seenIDs := make(map[string]bool, len(moments))
	seenSlugs := make(map[string]bool, len(moments))
	for i := range moments {
		moment := &moments[i]
		if seenIDs[moment.ID] {
			return fmt.Errorf("duplicate moment id %q", moment.ID)
		}
		if seenSlugs[moment.Slug] {
			return fmt.Errorf("duplicate moment slug %q", moment.Slug)
		}
		seenIDs[moment.ID] = true
		seenSlugs[moment.Slug] = true
		if err := ValidateMoment(*moment); err != nil {
			return err
		}
	}
	return nil
}

// ValidateMoment validates one completed scenario independently of pack-wide
// coverage. Draft tooling uses it before a scenario can be published.
func ValidateMoment(moment model.Moment) error {
	if moment.ID == "" || moment.Slug == "" || moment.Title == "" || moment.Description == "" || moment.Map == "" || moment.ControlledUnitID == "" {
		return fmt.Errorf("moment identity fields cannot be empty")
	}
	if !slugPattern.MatchString(moment.Slug) {
		return fmt.Errorf("moment %q has invalid slug %q", moment.ID, moment.Slug)
	}
	if moment.StartTimeSeconds < 0 || moment.MaxTurns < 1 || moment.MaxTurns > 20 || len(moment.ReasonTags) == 0 || len(moment.Units) < 2 {
		return fmt.Errorf("moment %q is not playable", moment.ID)
	}
	if len(moment.Units) > maxUnitsPerMoment {
		return fmt.Errorf("moment %q exceeds the %d-unit simulation limit", moment.ID, maxUnitsPerMoment)
	}
	if !normalizedSignals(moment.Signals) {
		return fmt.Errorf("moment %q has signals outside 0..1", moment.ID)
	}
	if err := validateReplayEvidence(moment); err != nil {
		return err
	}
	if moment.Rules.InitialAdvantage <= 0 || moment.Rules.InitialAdvantage >= 1 {
		return fmt.Errorf("moment %q has initial advantage outside 0..1", moment.ID)
	}

	controlledFound := false
	teams := map[string]bool{}
	teamCounts := map[string]int{}
	marksmen := map[string]int{}
	unitIDs := make(map[string]bool, len(moment.Units))
	unitTeams := make(map[string]string, len(moment.Units))
	for _, unit := range moment.Units {
		if unit.ID == "" || unit.Role == "" || unitIDs[unit.ID] {
			return fmt.Errorf("moment %q has an empty or duplicate unit id", moment.ID)
		}
		unitIDs[unit.ID] = true
		unitTeams[unit.ID] = unit.Team
		teams[unit.Team] = true
		teamCounts[unit.Team]++
		if unit.Class == model.ClassMarksman {
			marksmen[unit.Team]++
		}
		controlledFound = controlledFound || unit.ID == moment.ControlledUnitID && unit.Team == "blue" && unit.Policy == "controlled" && unit.Alive
		profile, classValid := model.Profile(unit.Class)
		if !classValid || unit.MaxHP != profile.MaxHP || unit.MoveRange != profile.MoveRange ||
			unit.MoveSpeed != profile.MoveRange || unit.AttackRange != profile.AttackRange {
			return fmt.Errorf("moment %q unit %q does not match class %q profile", moment.ID, unit.ID, unit.Class)
		}
		if !oneOf(unit.Team, "blue", "red") || !oneOf(unit.Policy, "controlled", "support", "protector", "aggressive", "skirmisher") ||
			!pointOnMap(unit.Position) || unit.HP < 0 || unit.HP > unit.MaxHP || unit.Alive != (unit.HP > 0) ||
			unit.AttackDamage <= 0 || unit.Armor < 0 || unit.VisionRange <= 0 ||
			unit.AttackCooldown < 1 || unit.Cooldown < 0 {
			return fmt.Errorf("moment %q has invalid combat state for unit %q", moment.ID, unit.ID)
		}
	}
	if !controlledFound {
		return fmt.Errorf("moment %q does not contain a live controlled unit %q", moment.ID, moment.ControlledUnitID)
	}
	if !teams["blue"] || !teams["red"] {
		return fmt.Errorf("moment %q must contain both teams", moment.ID)
	}
	if teamCounts["blue"] != 5 || teamCounts["red"] != 5 {
		return fmt.Errorf("moment %q must contain a full five-player roster for each team", moment.ID)
	}
	if marksmen["blue"] != 1 || marksmen["red"] != 1 {
		return fmt.Errorf("moment %q must contain exactly one marksman per team", moment.ID)
	}

	victory := moment.Rules.Victory
	if !oneOf(victory.Kind, "secure-objective", "eliminate-target", "skirmish") || victory.Description == "" || victory.DefeatDescription == "" {
		return fmt.Errorf("moment %q must define explicit victory and defeat conditions", moment.ID)
	}
	if victory.TargetUnitID != "" && !unitIDs[victory.TargetUnitID] {
		return fmt.Errorf("moment %q targets unknown unit %q", moment.ID, victory.TargetUnitID)
	}
	if victory.TargetUnitID != "" && unitTeams[victory.TargetUnitID] != "red" {
		return fmt.Errorf("moment %q target %q must be on the red team", moment.ID, victory.TargetUnitID)
	}
	if victory.AllowEscape && (!pointOnMap(victory.SafeZone) || victory.SafeRadius <= 0 || victory.EscapeTurns < 1) {
		return fmt.Errorf("moment %q has invalid escape rules", moment.ID)
	}
	scenarioElementIDs := make(map[string]bool, len(moment.Rules.Terrain)+1)
	if objective := moment.Rules.Objective; objective != nil {
		if objective.ID == "" || objective.Label == "" || !pointOnMap(objective.Position) || objective.Radius <= 0 || objective.CaptureTurns < 1 {
			return fmt.Errorf("moment %q has invalid objective rules", moment.ID)
		}
		scenarioElementIDs[objective.ID] = true
		if expected, fixed := canonicalObjectiveLandmarks[objective.ID]; fixed && objective.Position != expected {
			return fmt.Errorf("moment %q objective %q must remain at canonical position (%.0f, %.0f)", moment.ID, objective.ID, expected.X, expected.Y)
		}
	}
	if victory.Kind == "secure-objective" && moment.Rules.Objective == nil {
		return fmt.Errorf("moment %q requires objective rules", moment.ID)
	}
	if victory.Kind == "eliminate-target" && victory.TargetUnitID == "" {
		return fmt.Errorf("moment %q requires an elimination target", moment.ID)
	}
	riverFound := false
	for _, terrain := range moment.Rules.Terrain {
		if terrain.ID == "" || terrain.Label == "" || !oneOf(terrain.Kind, "river", "brush", "wall", "safe-zone") ||
			!pointOnMap(terrain.Position) || terrain.Radius <= 0 || terrain.MoveMultiplier <= 0 {
			return fmt.Errorf("moment %q has invalid terrain %q", moment.ID, terrain.ID)
		}
		if expected, fixed := canonicalTerrainLandmarks[terrain.ID]; fixed {
			if terrain.Position != expected {
				return fmt.Errorf("moment %q terrain %q must remain at canonical position (%.0f, %.0f)", moment.ID, terrain.ID, expected.X, expected.Y)
			}
			if terrain.Kind == "safe-zone" && (!victory.AllowEscape || victory.SafeZone != expected) {
				return fmt.Errorf("moment %q escape rules must use canonical terrain %q at (%.0f, %.0f)", moment.ID, terrain.ID, expected.X, expected.Y)
			}
		}
		scenarioElementIDs[terrain.ID] = true
		riverFound = riverFound || terrain.ID == "river"
	}
	if objective := moment.Rules.Objective; objective != nil && objective.ID == "river-core" && !riverFound {
		return fmt.Errorf("moment %q river core must be placed inside canonical river terrain", moment.ID)
	}
	if err := validateMechanicBriefing(moment, scenarioElementIDs); err != nil {
		return err
	}

	if len(moment.Rules.ReferencePlan) != moment.MaxTurns || len(moment.Rules.ReferenceReasons) != moment.MaxTurns {
		return fmt.Errorf("moment %q reference plan and reasons must cover exactly %d turns", moment.ID, moment.MaxTurns)
	}
	for _, action := range moment.Rules.ReferencePlan {
		if err := validateAction(action); err != nil {
			return fmt.Errorf("moment %q has invalid reference action: %w", moment.ID, err)
		}
	}
	for _, reason := range moment.Rules.ReferenceReasons {
		if strings.TrimSpace(reason) == "" {
			return fmt.Errorf("moment %q has a blank reference reason", moment.ID)
		}
	}
	for _, actionType := range actionTypes {
		action, ok := moment.Rules.ActionDefaults[actionType]
		if !ok || action.Type != actionType {
			return fmt.Errorf("moment %q must define a valid %q action default", moment.ID, actionType)
		}
		if err := validateAction(action); err != nil {
			return fmt.Errorf("moment %q has invalid %q action default: %w", moment.ID, actionType, err)
		}
		continuation, ok := moment.Rules.ReferenceContinuations[actionType]
		if !ok || len(continuation) != moment.MaxTurns-1 {
			return fmt.Errorf("moment %q must define %d %q continuation actions", moment.ID, moment.MaxTurns-1, actionType)
		}
		for _, next := range continuation {
			if err := validateAction(next); err != nil {
				return fmt.Errorf("moment %q has invalid %q continuation: %w", moment.ID, actionType, err)
			}
		}
	}
	return validateAuthoring(moment)
}

func validateMechanicBriefing(moment model.Moment, scenarioElementIDs map[string]bool) error {
	covered := make(map[string]bool)
	if briefing := moment.MechanicBriefing; briefing != nil {
		if len(briefing.Mechanics) == 0 {
			return fmt.Errorf("moment %q has an empty mechanic briefing", moment.ID)
		}
		for _, mechanic := range briefing.Mechanics {
			if strings.TrimSpace(mechanic.ElementID) == "" || strings.TrimSpace(mechanic.Name) == "" ||
				strings.TrimSpace(mechanic.Description) == "" || strings.TrimSpace(mechanic.RoleInScenario) == "" {
				return fmt.Errorf("moment %q has an incomplete mechanic briefing", moment.ID)
			}
			if !scenarioElementIDs[mechanic.ElementID] {
				return fmt.Errorf("moment %q mechanic briefing references unknown element %q", moment.ID, mechanic.ElementID)
			}
			if covered[mechanic.ElementID] {
				return fmt.Errorf("moment %q repeats mechanic briefing element %q", moment.ID, mechanic.ElementID)
			}
			covered[mechanic.ElementID] = true
		}
	}
	for elementID := range scenarioElementIDs {
		if scenarioSpecificMechanicIDs[elementID] && !covered[elementID] {
			return fmt.Errorf("moment %q must explain scenario-specific mechanic %q before play", moment.ID, elementID)
		}
	}
	return nil
}

func validateReplayEvidence(moment model.Moment) error {
	evidence := moment.ReplayEvidence
	if evidence == nil || strings.TrimSpace(evidence.BundleID) == "" || len(evidence.BundleSHA256) != 64 ||
		strings.TrimSpace(evidence.SourceMomentID) == "" || evidence.Game < 1 ||
		strings.TrimSpace(evidence.DecisionTime) == "" || evidence.SourceVODSeconds < 0 ||
		strings.TrimSpace(evidence.Judgment) == "" || strings.TrimSpace(evidence.Assessment) == "" ||
		strings.TrimSpace(evidence.CoachingCorrection) == "" || len(evidence.CaptionEvidence) == 0 ||
		len(evidence.ExternalEvidence) == 0 ||
		!strings.Contains(strings.ToLower(evidence.CoordinateMethod), "approx") {
		return fmt.Errorf("moment %q has incomplete replay-bundle evidence or does not label coordinates as approximations", moment.ID)
	}
	return nil
}

func validateAuthoring(moment model.Moment) error {
	authoring := moment.Authoring
	if !oneOf(authoring.Category, categories...) || !oneOf(authoring.SkillLevel, skillLevels...) || strings.TrimSpace(authoring.AnalystRationale) == "" {
		return fmt.Errorf("moment %q has invalid authoring category, skill level, or rationale", moment.ID)
	}
	if len(authoring.IntendedTradeoffs) < minimumTradeoffs {
		return fmt.Errorf("moment %q needs at least %d intended tradeoffs", moment.ID, minimumTradeoffs)
	}
	for _, tradeoff := range authoring.IntendedTradeoffs {
		if strings.TrimSpace(tradeoff) == "" {
			return fmt.Errorf("moment %q has a blank intended tradeoff", moment.ID)
		}
	}
	if len(authoring.PlausibleAlternatives) < minimumAlternates {
		return fmt.Errorf("moment %q needs at least %d plausible alternatives", moment.ID, minimumAlternates)
	}
	alternativeActions := map[string]bool{}
	for _, alternative := range authoring.PlausibleAlternatives {
		if err := validateAction(alternative.Action); err != nil || strings.TrimSpace(alternative.When) == "" || strings.TrimSpace(alternative.Tradeoff) == "" {
			return fmt.Errorf("moment %q has an invalid plausible alternative", moment.ID)
		}
		if alternativeActions[alternative.Action.Type] {
			return fmt.Errorf("moment %q repeats plausible alternative %q", moment.ID, alternative.Action.Type)
		}
		alternativeActions[alternative.Action.Type] = true
	}
	if len(authoring.AcceptanceTests) < minimumTests {
		return fmt.Errorf("moment %q needs at least %d acceptance tests", moment.ID, minimumTests)
	}
	testNames := map[string]bool{}
	expectedStatuses := map[string]bool{}
	for _, test := range authoring.AcceptanceTests {
		if strings.TrimSpace(test.Name) == "" || testNames[test.Name] || !oneOf(test.ExpectedStatus, "won", "lost") ||
			test.ExpectedTerminalTurn < 0 || test.ExpectedTerminalTurn > moment.MaxTurns ||
			len(test.Actions) < 1 || len(test.Actions) > moment.MaxTurns ||
			(test.ExpectedTerminalTurn > 0 && len(test.Actions) != test.ExpectedTerminalTurn) || strings.TrimSpace(test.ExpectedOutcomeContains) == "" {
			return fmt.Errorf("moment %q has invalid acceptance test %q", moment.ID, test.Name)
		}
		testNames[test.Name] = true
		expectedStatuses[test.ExpectedStatus] = true
		if len(test.DodgeBeforeTurns) > 2 {
			return fmt.Errorf("moment %q acceptance test %q uses more than two Dodge reactions", moment.ID, test.Name)
		}
		dodgeTurns := map[int]bool{}
		for _, turn := range test.DodgeBeforeTurns {
			if turn < 1 || turn > len(test.Actions) || dodgeTurns[turn] {
				return fmt.Errorf("moment %q acceptance test %q has an invalid Dodge turn", moment.ID, test.Name)
			}
			dodgeTurns[turn] = true
		}
		for _, action := range test.Actions {
			if err := validateAction(action); err != nil {
				return fmt.Errorf("moment %q acceptance test %q: %w", moment.ID, test.Name, err)
			}
		}
	}
	if !expectedStatuses["won"] || !expectedStatuses["lost"] {
		return fmt.Errorf("moment %q acceptance tests must cover both a win and a loss", moment.ID)
	}
	return nil
}

func validateAction(action model.Action) error {
	if !oneOf(action.Type, actionTypes...) {
		return fmt.Errorf("unknown action %q", action.Type)
	}
	if action.Type == "move" && (action.Target == nil || !pointOnMap(*action.Target)) {
		return fmt.Errorf("move requires a target inside the map")
	}
	if action.Type != "move" && action.Target != nil {
		return fmt.Errorf("%s cannot include a target", action.Type)
	}
	return nil
}

func normalizedSignals(signals model.Signals) bool {
	return inUnitRange(signals.WinProbabilitySwing) && inUnitRange(signals.EventDensity) &&
		inUnitRange(signals.EntityProximity) && inUnitRange(signals.ResourceAsymmetry)
}

func inUnitRange(value float64) bool {
	return value >= 0 && value <= 1
}

func pointOnMap(point model.Point) bool {
	return point.X >= 0 && point.X <= 100 && point.Y >= 0 && point.Y <= 100
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
