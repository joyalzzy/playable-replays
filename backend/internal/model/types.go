package model

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Unit struct {
	ID             string  `json:"id"`
	Team           string  `json:"team"`
	Role           string  `json:"role"`
	Policy         string  `json:"policy"`
	Position       Point   `json:"position"`
	HP             int     `json:"hp"`
	MaxHP          int     `json:"maxHp"`
	AttackRange    float64 `json:"attackRange"`
	AttackDamage   int     `json:"attackDamage"`
	MoveSpeed      float64 `json:"moveSpeed"`
	Armor          int     `json:"armor"`
	VisionRange    float64 `json:"visionRange"`
	AttackCooldown int     `json:"attackCooldown"`
	Cooldown       int     `json:"cooldownTurns"`
	Shield         int     `json:"shield"`
	Guarded        bool    `json:"guarded"`
	Visible        bool    `json:"visible"`
	Alive          bool    `json:"alive"`
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

type Moment struct {
	ID               string        `json:"id"`
	Slug             string        `json:"slug"`
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	Map              string        `json:"map"`
	StartTimeSeconds int           `json:"startTimeSeconds"`
	Seed             int64         `json:"seed"`
	MaxTurns         int           `json:"maxTurns"`
	ControlledUnitID string        `json:"controlledUnitId"`
	ReasonTags       []string      `json:"reasonTags"`
	Signals          Signals       `json:"signals"`
	Units            []Unit        `json:"units"`
	Rules            ScenarioRules `json:"rules"`
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
	ReasonTags  []string `json:"reasonTags"`
	Score       float64  `json:"highlightScore"`
}

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
