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
	ID          string    `json:"id"`
	Team        string    `json:"team"`
	Role        string    `json:"role"`
	Class       UnitClass `json:"class"`
	Position    Point     `json:"position"`
	HP          int       `json:"hp"`
	MaxHP       int       `json:"maxHp"`
	MoveRange   float64   `json:"moveRange"`
	AttackRange float64   `json:"attackRange"`
	Cooldown    int       `json:"cooldownTurns"`
	Visible     bool      `json:"visible"`
	Alive       bool      `json:"alive"`
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
	ID               string     `json:"id"`
	MomentID         string     `json:"momentId"`
	ControlledUnitID string     `json:"controlledUnitId"`
	Turn             int        `json:"turn"`
	MaxTurns         int        `json:"maxTurns"`
	Status           string     `json:"status"`
	Score            int        `json:"score"`
	WinProbability   float64    `json:"winProbability"`
	ReferenceAction  Action     `json:"referenceAction"`
	LegalActions     []string   `json:"legalActions"`
	Units            []Unit     `json:"units"`
	Log              []LogEntry `json:"log"`
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
