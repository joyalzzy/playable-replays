package model

import (
	"math"
	"strings"
)

type UnitClass string

const (
	ClassTank     UnitClass = "tank"
	ClassFighter  UnitClass = "fighter"
	ClassMarksman UnitClass = "marksman"
	ClassMage     UnitClass = "mage"
	ClassSupport  UnitClass = "support"
	ClassAssassin UnitClass = "assassin"
)

type ClassProfile struct {
	MaxHP       int
	MoveRange   float64
	AttackRange float64
}

var classProfiles = map[UnitClass]ClassProfile{
	ClassTank:     {MaxHP: 160, MoveRange: 7, AttackRange: 10},
	ClassFighter:  {MaxHP: 125, MoveRange: 10, AttackRange: 14},
	ClassMarksman: {MaxHP: 90, MoveRange: 11, AttackRange: 28},
	ClassMage:     {MaxHP: 95, MoveRange: 9, AttackRange: 24},
	ClassSupport:  {MaxHP: 110, MoveRange: 8, AttackRange: 20},
	ClassAssassin: {MaxHP: 100, MoveRange: 13, AttackRange: 12},
}

func Profile(class UnitClass) (ClassProfile, bool) {
	profile, ok := classProfiles[class]
	return profile, ok
}

// ResolveClass keeps old fixtures playable while making class the authoritative
// gameplay field. Unknown legacy roles use the general-purpose fighter profile.
func ResolveClass(class UnitClass, legacyRole string) UnitClass {
	if _, ok := Profile(class); ok {
		return class
	}
	switch strings.ToLower(strings.TrimSpace(legacyRole)) {
	case "tank":
		return ClassTank
	case "carry", "adc", "marksman":
		return ClassMarksman
	case "mage", "mid", "midlaner":
		return ClassMage
	case "support":
		return ClassSupport
	case "assassin":
		return ClassAssassin
	case "fighter", "jungle", "jungler", "top", "toplaner":
		return ClassFighter
	default:
		return ClassFighter
	}
}

// ApplyClassProfile normalizes health proportionally and overwrites all class
// derived stats so fixture or model input can never grant illegal capabilities.
func ApplyClassProfile(unit Unit) Unit {
	unit.Class = ResolveClass(unit.Class, unit.Role)
	profile, _ := Profile(unit.Class)
	oldMaxHP := unit.MaxHP
	if oldMaxHP <= 0 {
		oldMaxHP = profile.MaxHP
	}
	if unit.HP > 0 && oldMaxHP != profile.MaxHP {
		unit.HP = int(math.Round(float64(unit.HP) / float64(oldMaxHP) * float64(profile.MaxHP)))
	}
	unit.MaxHP = profile.MaxHP
	unit.MoveRange = profile.MoveRange
	unit.AttackRange = profile.AttackRange
	if unit.HP > unit.MaxHP {
		unit.HP = unit.MaxHP
	}
	if unit.Role == "" {
		unit.Role = string(unit.Class)
	}
	return unit
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Unit struct {
	ID             string    `json:"id"`
	Team           string    `json:"team"`
	Role           string    `json:"role"`
	Class          UnitClass `json:"class"`
	Policy         string    `json:"policy"`
	Position       Point     `json:"position"`
	HP             int       `json:"hp"`
	MaxHP          int       `json:"maxHp"`
	MoveRange      float64   `json:"moveRange"`
	AttackRange    float64   `json:"attackRange"`
	AttackDamage   int       `json:"attackDamage"`
	MoveSpeed      float64   `json:"moveSpeed"`
	Armor          int       `json:"armor"`
	VisionRange    float64   `json:"visionRange"`
	AttackCooldown int       `json:"attackCooldown"`
	Cooldown       int       `json:"cooldownTurns"`
	Shield         int       `json:"shield"`
	Guarded        bool      `json:"guarded"`
	Visible        bool      `json:"visible"`
	Alive          bool      `json:"alive"`
}

type TerrainFeature struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	Kind           string  `json:"kind"`
	Position       Point   `json:"position"`
	Radius         float64 `json:"radius"`
	MoveMultiplier float64 `json:"moveMultiplier"`
	BlocksVision   bool    `json:"blocksVision"`
}

type ObjectiveRules struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Position     Point   `json:"position"`
	Radius       float64 `json:"radius"`
	CaptureTurns int     `json:"captureTurns"`
}

type VictoryRules struct {
	Kind              string  `json:"kind"`
	TargetUnitID      string  `json:"targetUnitId,omitempty"`
	Description       string  `json:"description"`
	DefeatDescription string  `json:"defeatDescription"`
	AllowEscape       bool    `json:"allowEscape"`
	SafeZone          Point   `json:"safeZone"`
	SafeRadius        float64 `json:"safeRadius"`
	EscapeTurns       int     `json:"escapeTurns"`
}

type ScenarioRules struct {
	InitialAdvantage       float64             `json:"initialAdvantage"`
	Objective              *ObjectiveRules     `json:"objective,omitempty"`
	Victory                VictoryRules        `json:"victory"`
	Terrain                []TerrainFeature    `json:"terrain"`
	ReferencePlan          []Action            `json:"referencePlan"`
	ReferenceReasons       []string            `json:"referenceReasons"`
	ReferenceContinuations map[string][]Action `json:"referenceContinuations"`
	ActionDefaults         map[string]Action   `json:"actionDefaults"`
}

type Signals struct {
	WinProbabilitySwing float64 `json:"winProbabilitySwing"`
	EventDensity        float64 `json:"eventDensity"`
	EntityProximity     float64 `json:"entityProximity"`
	ResourceAsymmetry   float64 `json:"resourceAsymmetry"`
}

// ReplayEvidence records the public evidence used to author a scenario. The
// normalized map coordinates remain explicit analyst approximations because
// the source bundle contains review frames and event timings, not positional
// telemetry.
type ReplayEvidence struct {
	BundleID           string   `json:"bundleId"`
	BundleSHA256       string   `json:"bundleSha256"`
	SourceMomentID     string   `json:"sourceMomentId"`
	Game               int      `json:"game"`
	DecisionTime       string   `json:"decisionTime"`
	SourceVODSeconds   float64  `json:"sourceVodSeconds"`
	Judgment           string   `json:"judgment"`
	Assessment         string   `json:"assessment"`
	CoachingCorrection string   `json:"coachingCorrection"`
	CaptionEvidence    []string `json:"captionEvidence"`
	ExternalEvidence   []string `json:"externalEvidence"`
	CoordinateMethod   string   `json:"coordinateMethod"`
}

type ScenarioAlternative struct {
	Action   Action `json:"action"`
	When     string `json:"when"`
	Tradeoff string `json:"tradeoff"`
}

type ScenarioAcceptanceTest struct {
	Name                    string   `json:"name"`
	Actions                 []Action `json:"actions"`
	DodgeBeforeTurns        []int    `json:"dodgeBeforeTurns"`
	ExpectedStatus          string   `json:"expectedStatus"`
	ExpectedTerminalTurn    int      `json:"expectedTerminalTurn"`
	ExpectedOutcomeContains string   `json:"expectedOutcomeContains"`
}

type ScenarioAuthoring struct {
	Category              string                   `json:"category"`
	SkillLevel            string                   `json:"skillLevel"`
	AnalystRationale      string                   `json:"analystRationale"`
	IntendedTradeoffs     []string                 `json:"intendedTradeoffs"`
	PlausibleAlternatives []ScenarioAlternative    `json:"plausibleAlternatives"`
	AcceptanceTests       []ScenarioAcceptanceTest `json:"acceptanceTests"`
}

// ScenarioMechanic explains a scenario-specific map element before the player
// is allowed to make a decision. ElementID links the explanation to the
// objective or terrain feature that implements the mechanic.
type ScenarioMechanic struct {
	ElementID      string `json:"elementId"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	RoleInScenario string `json:"roleInScenario"`
}

type MechanicBriefing struct {
	Mechanics []ScenarioMechanic `json:"mechanics"`
}

type Moment struct {
	ID               string            `json:"id"`
	Slug             string            `json:"slug"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	Map              string            `json:"map"`
	StartTimeSeconds int               `json:"startTimeSeconds"`
	Seed             int64             `json:"seed"`
	MaxTurns         int               `json:"maxTurns"`
	ControlledUnitID string            `json:"controlledUnitId"`
	ReasonTags       []string          `json:"reasonTags"`
	Signals          Signals           `json:"signals"`
	ReplayEvidence   *ReplayEvidence   `json:"replayEvidence,omitempty"`
	MechanicBriefing *MechanicBriefing `json:"mechanicBriefing,omitempty"`
	Units            []Unit            `json:"units"`
	Rules            ScenarioRules     `json:"rules"`
	Authoring        ScenarioAuthoring `json:"authoring"`
}

type Action struct {
	Type   string `json:"type"`
	Target *Point `json:"target,omitempty"`
}

type TurnRequest struct {
	Action Action `json:"action"`
}

type LogEntry struct {
	Turn     int    `json:"turn"`
	Actor    string `json:"actor"`
	Kind     string `json:"kind"`
	Action   string `json:"action"`
	ActorID  string `json:"actorId,omitempty"`
	TargetID string `json:"targetId,omitempty"`
	Value    int    `json:"value,omitempty"`
	Message  string `json:"message"`
}

type ObjectiveState struct {
	ID               string  `json:"id"`
	Label            string  `json:"label"`
	Position         Point   `json:"position"`
	Radius           float64 `json:"radius"`
	BlueProgress     int     `json:"blueProgress"`
	RedProgress      int     `json:"redProgress"`
	RequiredProgress int     `json:"requiredProgress"`
	Status           string  `json:"status"`
}

type Turret struct {
	ID       string `json:"id"`
	Team     string `json:"team"`
	Lane     string `json:"lane"`
	Position Point  `json:"position"`
	HP       int    `json:"hp"`
	MaxHP    int    `json:"maxHp"`
	Alive    bool   `json:"alive"`
}

// Projectile is a one-turn marksman skillshot. Position is its launch point;
// Target is the fixed point the shot was aimed at. The authoritative engine
// resolves or evades it before the next tactical action.
type Projectile struct {
	ID           string `json:"id"`
	Team         string `json:"team"`
	SourceUnitID string `json:"sourceUnitId"`
	TargetUnitID string `json:"targetUnitId"`
	Position     Point  `json:"position"`
	Target       Point  `json:"target"`
	Damage       int    `json:"damage"`
}

type BotControlState struct {
	Source       string `json:"source"`
	ModelName    string `json:"modelName,omitempty"`
	ModelVersion string `json:"modelVersion,omitempty"`
}

type ReferenceOutcome struct {
	FirstAction   Action   `json:"firstAction"`
	Status        string   `json:"status"`
	Turns         int      `json:"turns"`
	Advantage     float64  `json:"advantage"`
	OutcomeReason string   `json:"outcomeReason"`
	KeyEvents     []string `json:"keyEvents"`
}

type BestCaseAlternative struct {
	Action        Action  `json:"action"`
	Status        string  `json:"status"`
	Turns         int     `json:"turns"`
	Advantage     float64 `json:"advantage"`
	OutcomeReason string  `json:"outcomeReason"`
}

type BestCaseStep struct {
	Turn            int                   `json:"turn"`
	Action          Action                `json:"action"`
	Reason          string                `json:"reason"`
	AdvantageBefore float64               `json:"advantageBefore"`
	AdvantageAfter  float64               `json:"advantageAfter"`
	KeyEvents       []string              `json:"keyEvents"`
	Alternatives    []BestCaseAlternative `json:"alternatives"`
}

type BestCaseLine struct {
	Status        string         `json:"status"`
	Turns         int            `json:"turns"`
	Advantage     float64        `json:"advantage"`
	OutcomeReason string         `json:"outcomeReason"`
	Method        string         `json:"method"`
	Steps         []BestCaseStep `json:"steps"`
}

type Session struct {
	ID                  string             `json:"id"`
	MomentID            string             `json:"momentId"`
	ControlledUnitID    string             `json:"controlledUnitId"`
	ScenarioGoal        string             `json:"scenarioGoal"`
	MechanicBriefing    *MechanicBriefing  `json:"mechanicBriefing,omitempty"`
	Turn                int                `json:"turn"`
	MaxTurns            int                `json:"maxTurns"`
	Status              string             `json:"status"`
	OutcomeReason       string             `json:"outcomeReason,omitempty"`
	Advantage           float64            `json:"advantage"`
	EscapeProgress      int                `json:"escapeProgress"`
	EscapeTurnsRequired int                `json:"escapeTurnsRequired"`
	VisibleEnemyCount   int                `json:"visibleEnemyCount"`
	UnknownEnemyCount   int                `json:"unknownEnemyCount"`
	VisionLimited       bool               `json:"visionLimited"`
	Objective           *ObjectiveState    `json:"objective,omitempty"`
	Terrain             []TerrainFeature   `json:"terrain"`
	Turrets             []Turret           `json:"turrets"`
	Projectiles         []Projectile       `json:"projectiles"`
	DodgeCharges        int                `json:"dodgeCharges"`
	DodgeAvailable      bool               `json:"dodgeAvailable"`
	BotControl          BotControlState    `json:"botControl"`
	LastReferenceAction *Action            `json:"lastReferenceAction,omitempty"`
	ReferenceReason     string             `json:"referenceReason,omitempty"`
	ReferenceOutcomes   []ReferenceOutcome `json:"referenceOutcomes,omitempty"`
	BestCase            *BestCaseLine      `json:"bestCase,omitempty"`
	LegalActions        []string           `json:"legalActions"`
	Units               []Unit             `json:"units"`
	Log                 []LogEntry         `json:"log"`
	Debrief             []string           `json:"debrief,omitempty"`
}

type CreateSessionRequest struct {
	MomentID string `json:"momentId"`
}

type MomentSummary struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Map         string   `json:"map"`
	Category    string   `json:"category"`
	SkillLevel  string   `json:"skillLevel"`
	ReasonTags  []string `json:"reasonTags"`
	Score       float64  `json:"highlightScore"`
}

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
