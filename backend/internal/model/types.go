package model

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Unit struct {
	ID       string `json:"id"`
	Team     string `json:"team"`
	Role     string `json:"role"`
	Position Point  `json:"position"`
	HP       int    `json:"hp"`
	MaxHP    int    `json:"maxHp"`
	Cooldown int    `json:"cooldownTurns"`
	Visible  bool   `json:"visible"`
	Alive    bool   `json:"alive"`
}

type Signals struct {
	WinProbabilitySwing float64 `json:"winProbabilitySwing"`
	EventDensity        float64 `json:"eventDensity"`
	EntityProximity     float64 `json:"entityProximity"`
	ResourceAsymmetry   float64 `json:"resourceAsymmetry"`
}

type Moment struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Map              string   `json:"map"`
	StartTimeSeconds int      `json:"startTimeSeconds"`
	Seed             int64    `json:"seed"`
	MaxTurns         int      `json:"maxTurns"`
	ControlledUnitID string   `json:"controlledUnitId"`
	ReasonTags       []string `json:"reasonTags"`
	Signals          Signals  `json:"signals"`
	Units            []Unit   `json:"units"`
}

type Action struct {
	Type   string `json:"type"`
	Target *Point `json:"target,omitempty"`
}

type TurnRequest struct {
	Action Action `json:"action"`
}

type LogEntry struct {
	Turn    int    `json:"turn"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Message string `json:"message"`
}

type Session struct {
	ID              string     `json:"id"`
	MomentID        string     `json:"momentId"`
	Turn            int        `json:"turn"`
	MaxTurns        int        `json:"maxTurns"`
	Status          string     `json:"status"`
	Score           int        `json:"score"`
	WinProbability  float64    `json:"winProbability"`
	ReferenceAction Action     `json:"referenceAction"`
	LegalActions    []string   `json:"legalActions"`
	Units           []Unit     `json:"units"`
	Log             []LogEntry `json:"log"`
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
