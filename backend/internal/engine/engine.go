package engine

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

var ErrIllegalAction = errors.New("illegal action")

var actionTypes = []string{"move", "hold", "contest", "retreat"}

type Engine struct {
	moment            model.Moment
	session           model.Session
	rng               *rand.Rand
	referenceOutcomes []model.ReferenceOutcome
	bestCase          *model.BestCaseLine
}

func New(moment model.Moment, sessionID string) *Engine {
	return newEngine(moment, sessionID, true)
}

func newEngine(moment model.Moment, sessionID string, withReferences bool) *Engine {
	e := &Engine{moment: normalizeMoment(cloneMoment(moment))}
	e.Reset(sessionID)
	if withReferences {
		e.referenceOutcomes = e.computeReferenceOutcomes()
		e.bestCase = e.computeBestCase()
	}
	return e
}

func (e *Engine) Reset(sessionID string) model.Session {
	e.rng = rand.New(rand.NewSource(e.moment.Seed))
	e.session = model.Session{
		ID:                  sessionID,
		MomentID:            e.moment.ID,
		ControlledUnitID:    e.moment.ControlledUnitID,
		ScenarioGoal:        e.moment.Rules.Victory.Description,
		MaxTurns:            e.moment.MaxTurns,
		Status:              "active",
		Advantage:           e.moment.Rules.InitialAdvantage,
		EscapeTurnsRequired: e.moment.Rules.Victory.EscapeTurns,
		Terrain:             slices.Clone(e.moment.Rules.Terrain),
		LegalActions:        slices.Clone(actionTypes),
		Units:               cloneUnits(e.moment.Units),
		Log:                 []model.LogEntry{},
	}
	if objective := e.moment.Rules.Objective; objective != nil {
		e.session.Objective = &model.ObjectiveState{
			ID: objective.ID, Label: objective.Label, Position: objective.Position,
			Radius: objective.Radius, RequiredProgress: objective.CaptureTurns, Status: "neutral",
		}
	}
	e.applyFog(false)
	e.recomputeAdvantage()
	return e.State()
}

func (e *Engine) State() model.Session {
	state := e.session
	state.Units = make([]model.Unit, 0, len(e.session.Units))
	for _, unit := range e.session.Units {
		if unit.Team == "blue" || unit.Visible {
			state.Units = append(state.Units, unit)
		}
	}
	state.Log = slices.Clone(e.session.Log)
	state.LegalActions = slices.Clone(e.session.LegalActions)
	state.Terrain = slices.Clone(e.session.Terrain)
	state.Debrief = slices.Clone(e.session.Debrief)
	if e.session.Objective != nil {
		objective := *e.session.Objective
		state.Objective = &objective
	}
	if e.session.LastReferenceAction != nil {
		action := cloneAction(*e.session.LastReferenceAction)
		state.LastReferenceAction = &action
	}
	if e.session.Status != "active" {
		state.ReferenceOutcomes = cloneReferenceOutcomes(e.referenceOutcomes)
		state.BestCase = cloneBestCase(e.bestCase)
	} else {
		state.ReferenceOutcomes = nil
		state.BestCase = nil
	}
	return state
}

func (e *Engine) Apply(action model.Action) (model.Session, error) {
	if e.session.Status != "active" {
		return e.State(), fmt.Errorf("%w: session is complete", ErrIllegalAction)
	}
	if !slices.Contains(e.session.LegalActions, action.Type) {
		return e.State(), fmt.Errorf("%w: unknown action %q", ErrIllegalAction, action.Type)
	}
	if action.Type == "move" && action.Target == nil {
		return e.State(), fmt.Errorf("%w: move requires a target", ErrIllegalAction)
	}
	if action.Target != nil && !pointOnMap(*action.Target) {
		return e.State(), fmt.Errorf("%w: target is outside the map", ErrIllegalAction)
	}

	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return e.State(), fmt.Errorf("%w: controlled unit is unavailable", ErrIllegalAction)
	}

	e.beginTurn()
	e.session.Turn++
	e.resolveUser(controlled, action)
	e.applyFog(true)
	e.resolveAllies(controlled)
	e.applyFog(true)
	e.resolveEnemies()
	e.applyFog(true)
	e.updateObjective()
	e.updateEscape()
	e.recomputeAdvantage()
	e.evaluateOutcome()
	e.revealReferenceForTurn()
	if e.session.Status != "active" {
		e.session.Debrief = e.buildDebrief()
	}
	return e.State(), nil
}

func (e *Engine) beginTurn() {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		unit.Guarded = false
		unit.Shield = 0
		if unit.Cooldown > 0 {
			unit.Cooldown--
		}
	}
}

func (e *Engine) resolveUser(unit *model.Unit, action model.Action) {
	switch action.Type {
	case "move":
		e.moveUnit(unit, *action.Target, 1, "user", "reposition")
	case "hold":
		unit.Guarded = true
		unit.Shield = 4
		e.addLog("user", "defense", "hold", unit.ID, unit.ID, 4,
			fmt.Sprintf("%s held formation, gaining 4 shield and reducing incoming damage this turn.", unitName(*unit)))
	case "contest":
		target := e.nearestVisibleEnemy(*unit)
		if target == nil {
			if objective := e.moment.Rules.Objective; objective != nil {
				e.moveUnit(unit, objective.Position, 1, "user", "contest")
				e.addLog("user", "objective", "contest", unit.ID, objective.ID, 0,
					fmt.Sprintf("%s advanced on %s because no enemy target was visible.", unitName(*unit), objective.Label))
			} else {
				unit.Guarded = true
				e.addLog("user", "defense", "contest", unit.ID, "", 0,
					fmt.Sprintf("%s found no visible target and held position.", unitName(*unit)))
			}
			return
		}
		if distance(unit.Position, target.Position) > unit.AttackRange {
			e.moveUnit(unit, target.Position, 1, "user", "close distance")
		}
		if distance(unit.Position, target.Position) <= unit.AttackRange {
			e.attack(unit, target, "user", "contest")
		} else {
			e.addLog("user", "position", "contest", unit.ID, target.ID, 0,
				fmt.Sprintf("%s closed distance but %s remained out of range.", unitName(*unit), unitName(*target)))
		}
	case "retreat":
		unit.Guarded = true
		destination := e.moment.Rules.Victory.SafeZone
		e.moveUnit(unit, destination, 1.2, "user", "retreat")
		e.addLog("user", "defense", "retreat", unit.ID, "", 0,
			fmt.Sprintf("%s disengaged toward the safe zone and reduced incoming damage this turn.", unitName(*unit)))
	}
}

func (e *Engine) resolveAllies(controlled *model.Unit) {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.Team != "blue" || unit.ID == controlled.ID {
			continue
		}
		switch unit.Policy {
		case "support":
			if unit.Cooldown == 0 && distance(unit.Position, controlled.Position) <= unit.VisionRange {
				shield := 10
				controlled.Shield += shield
				unit.Cooldown = unit.AttackCooldown
				e.addLog("ally", "support", "shield", unit.ID, controlled.ID, shield,
					fmt.Sprintf("%s shielded %s for %d.", unitName(*unit), unitName(*controlled), shield))
			} else {
				e.moveUnit(unit, controlled.Position, .85, "ally", "follow")
			}
		case "protector":
			target := e.nearestVisibleEnemy(*unit)
			if target != nil && distance(target.Position, controlled.Position) <= 30 {
				if distance(unit.Position, target.Position) > unit.AttackRange {
					e.moveUnit(unit, target.Position, .9, "ally", "intercept")
				}
				if distance(unit.Position, target.Position) <= unit.AttackRange {
					e.attack(unit, target, "ally", "intercept")
				}
			} else {
				e.moveUnit(unit, controlled.Position, .75, "ally", "protect")
			}
		default:
			target := e.nearestVisibleEnemy(*unit)
			if target != nil && distance(unit.Position, target.Position) <= unit.VisionRange {
				if distance(unit.Position, target.Position) > unit.AttackRange {
					e.moveUnit(unit, target.Position, .85, "ally", "engage")
				}
				if distance(unit.Position, target.Position) <= unit.AttackRange {
					e.attack(unit, target, "ally", "engage")
				}
			} else {
				e.moveUnit(unit, controlled.Position, .75, "ally", "regroup")
			}
		}
	}
}

func (e *Engine) resolveEnemies() {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.Team != "red" {
			continue
		}
		if unit.Policy == "support" {
			if e.resolveEnemySupport(unit) {
				continue
			}
		}
		target := e.enemyTarget(*unit)
		if target == nil {
			if objective := e.moment.Rules.Objective; objective != nil {
				e.moveUnit(unit, objective.Position, .8, "enemy", "objective")
			}
			continue
		}
		currentDistance := distance(unit.Position, target.Position)
		if unit.Policy == "skirmisher" && currentDistance < unit.AttackRange*.5 {
			e.moveUnit(unit, moveAway(unit.Position, target.Position, unit.MoveSpeed), 1, "enemy", "kite")
			currentDistance = distance(unit.Position, target.Position)
		}
		if currentDistance > unit.AttackRange {
			e.moveUnit(unit, target.Position, 1, "enemy", "engage")
		}
		if distance(unit.Position, target.Position) <= unit.AttackRange {
			e.attack(unit, target, "enemy", "attack")
		}
	}
}

func (e *Engine) resolveEnemySupport(unit *model.Unit) bool {
	var weakest *model.Unit
	lowestRatio := 2.0
	for i := range e.session.Units {
		candidate := &e.session.Units[i]
		if candidate.Team != "red" || !candidate.Alive || candidate.ID == unit.ID ||
			distance(unit.Position, candidate.Position) > unit.VisionRange {
			continue
		}
		ratio := float64(candidate.HP) / float64(candidate.MaxHP)
		if ratio < lowestRatio {
			weakest, lowestRatio = candidate, ratio
		}
	}
	if weakest != nil && unit.Cooldown == 0 && lowestRatio < .9 {
		weakest.Shield += 9
		unit.Cooldown = unit.AttackCooldown
		if unit.Visible || weakest.Visible {
			e.addLog("enemy", "support", "shield", unit.ID, weakest.ID, 9,
				fmt.Sprintf("%s shielded %s for 9.", unitName(*unit), unitName(*weakest)))
		}
		return true
	}
	return false
}

func (e *Engine) moveUnit(unit *model.Unit, destination model.Point, speedScale float64, actor, action string) {
	if !unit.Alive {
		return
	}
	public := unit.Team == "blue" || unit.Visible
	start := unit.Position
	limit := unit.MoveSpeed * speedScale * e.movementMultiplier(start)
	unit.Position = clampPoint(moveToward(start, destination, limit))
	moved := distance(start, unit.Position)
	if moved < .05 || (actor == "enemy" && !public) {
		return
	}
	e.addLog(actor, "movement", action, unit.ID, "", 0,
		fmt.Sprintf("%s moved %.1f map units toward (%.0f, %.0f).", unitName(*unit), moved, destination.X, destination.Y))
}

func (e *Engine) attack(attacker, target *model.Unit, actor, action string) bool {
	if attacker.Cooldown > 0 || !attacker.Alive || !target.Alive {
		return false
	}
	raw := max(1, attacker.AttackDamage+e.rng.Intn(5)-2)
	damage := int(math.Round(float64(raw) * 100 / float64(100+max(0, target.Armor))))
	if target.Guarded {
		damage = max(1, int(math.Round(float64(damage)*.65)))
	}
	absorbed := min(target.Shield, damage)
	target.Shield -= absorbed
	damage -= absorbed
	before := target.HP
	target.HP = max(0, target.HP-damage)
	target.Alive = target.HP > 0
	attacker.Cooldown = attacker.AttackCooldown

	attackerLabel := unitName(*attacker)
	actorID := attacker.ID
	if attacker.Team == "red" && !attacker.Visible {
		attackerLabel = "An unseen threat"
		actorID = ""
	}
	message := fmt.Sprintf("%s hit %s for %d damage (%d to %d HP).", attackerLabel, unitName(*target), damage, before, target.HP)
	if absorbed > 0 {
		message = fmt.Sprintf("%s hit %s: %d shield absorbed and %d HP damage (%d to %d HP).",
			attackerLabel, unitName(*target), absorbed, damage, before, target.HP)
	}
	e.addLog(actor, "damage", action, actorID, target.ID, damage, message)
	if !target.Alive {
		e.addLog("system", "elimination", "eliminated", attacker.ID, target.ID, 0,
			fmt.Sprintf("%s was eliminated by %s.", unitName(*target), attackerLabel))
	}
	return true
}

func (e *Engine) updateObjective() {
	state := e.session.Objective
	rules := e.moment.Rules.Objective
	if state == nil || rules == nil {
		return
	}
	blue, red := 0, 0
	for _, unit := range e.session.Units {
		if !unit.Alive || distance(unit.Position, rules.Position) > rules.Radius {
			continue
		}
		if unit.Team == "blue" {
			blue++
		} else {
			red++
		}
	}
	previousBlue, previousRed := state.BlueProgress, state.RedProgress
	switch {
	case blue > 0 && red > 0:
		state.Status = "contested"
	case blue > 0:
		state.BlueProgress++
		state.RedProgress = max(0, state.RedProgress-1)
		state.Status = "blue-control"
	case red > 0:
		state.RedProgress++
		state.BlueProgress = max(0, state.BlueProgress-1)
		state.Status = "red-control"
	default:
		state.Status = "neutral"
	}
	state.BlueProgress = min(state.RequiredProgress, state.BlueProgress)
	state.RedProgress = min(state.RequiredProgress, state.RedProgress)
	if state.BlueProgress >= state.RequiredProgress {
		state.Status = "secured-blue"
	} else if state.RedProgress >= state.RequiredProgress {
		state.Status = "secured-red"
	}
	if previousBlue != state.BlueProgress || previousRed != state.RedProgress || state.Status == "contested" {
		e.addLog("system", "objective", "control", state.ID, "", 0,
			fmt.Sprintf("%s is %s: blue %d/%d, red %d/%d (%d blue and %d red units in range).",
				state.Label, strings.ReplaceAll(state.Status, "-", " "), state.BlueProgress, state.RequiredProgress,
				state.RedProgress, state.RequiredProgress, blue, red))
	}
}

func (e *Engine) updateEscape() {
	rules := e.moment.Rules.Victory
	if !rules.AllowEscape || rules.EscapeTurns < 1 {
		return
	}
	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return
	}
	if distance(controlled.Position, rules.SafeZone) > rules.SafeRadius {
		e.session.EscapeProgress = 0
		return
	}
	threatened := false
	for _, unit := range e.session.Units {
		if unit.Team == "red" && unit.Alive && distance(unit.Position, controlled.Position) <= 18 {
			threatened = true
			break
		}
	}
	if threatened {
		e.session.EscapeProgress = 0
		e.addLog("system", "escape", "contested", controlled.ID, "", 0,
			"The safe zone is contested; escape progress reset.")
		return
	}
	e.session.EscapeProgress = min(rules.EscapeTurns, e.session.EscapeProgress+1)
	e.addLog("system", "escape", "progress", controlled.ID, "", e.session.EscapeProgress,
		fmt.Sprintf("Escape stabilized for %d/%d required turns.", e.session.EscapeProgress, rules.EscapeTurns))
}

func (e *Engine) evaluateOutcome() {
	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		e.finish("lost", e.moment.Rules.Victory.DefeatDescription)
		return
	}
	if e.moment.Rules.Victory.AllowEscape && e.session.EscapeProgress >= e.moment.Rules.Victory.EscapeTurns {
		e.finish("won", "The controlled unit disengaged and held the safe route long enough to escape.")
		return
	}
	if state := e.session.Objective; state != nil {
		if state.BlueProgress >= state.RequiredProgress {
			e.finish("won", fmt.Sprintf("Your team secured %s before the opponents completed control.", state.Label))
			return
		}
		if state.RedProgress >= state.RequiredProgress {
			e.finish("lost", fmt.Sprintf("The opposing team secured %s first.", state.Label))
			return
		}
	}
	if targetID := e.moment.Rules.Victory.TargetUnitID; targetID != "" {
		if target := e.unit(targetID); target == nil || !target.Alive {
			e.finish("won", fmt.Sprintf("The isolated %s was eliminated before reinforcements stabilized the fight.", unitRole(targetID, e.session.Units)))
			return
		}
	}
	if allTeamEliminated(e.session.Units, "red") {
		e.finish("won", "Every opposing unit was eliminated.")
		return
	}
	if e.session.Turn < e.session.MaxTurns {
		return
	}
	switch e.moment.Rules.Victory.Kind {
	case "secure-objective":
		e.finish("lost", "The objective window closed before your team secured control or completed an escape.")
	case "eliminate-target":
		e.finish("lost", "The overextended target survived until reinforcements stabilized the position.")
	default:
		if e.session.Advantage >= .55 {
			e.finish("won", "Your team ended the decision window with the stronger tactical state.")
		} else {
			e.finish("lost", "Your team ended the decision window under greater pressure.")
		}
	}
}

func (e *Engine) finish(status, reason string) {
	if e.session.Status != "active" {
		return
	}
	e.session.Status = status
	e.session.OutcomeReason = reason
	if status == "won" {
		e.session.Advantage = max(e.session.Advantage, .78)
	} else {
		e.session.Advantage = min(e.session.Advantage, .22)
	}
	e.addLog("system", "outcome", status, "", "", 0, reason)
}

func (e *Engine) recomputeAdvantage() {
	blueHealth, redHealth := teamHealthRatio(e.session.Units, "blue"), teamHealthRatio(e.session.Units, "red")
	blueAlive, blueTotal := teamAlive(e.session.Units, "blue")
	redAlive, redTotal := teamAlive(e.session.Units, "red")
	aliveDelta := float64(blueAlive)/float64(max(1, blueTotal)) - float64(redAlive)/float64(max(1, redTotal))
	advantage := e.moment.Rules.InitialAdvantage + .24*(blueHealth-redHealth) + .14*aliveDelta
	if objective := e.session.Objective; objective != nil {
		advantage += .16 * float64(objective.BlueProgress-objective.RedProgress) / float64(max(1, objective.RequiredProgress))
	}
	if targetID := e.moment.Rules.Victory.TargetUnitID; targetID != "" {
		if target := e.unit(targetID); target != nil {
			advantage += .12 * (1 - float64(target.HP)/float64(target.MaxHP))
		} else {
			advantage += .12
		}
	}
	if e.moment.Rules.Victory.AllowEscape && e.moment.Rules.Victory.EscapeTurns > 0 {
		advantage += .08 * float64(e.session.EscapeProgress) / float64(e.moment.Rules.Victory.EscapeTurns)
	}
	e.session.Advantage = clamp(advantage, .05, .95)
}

func (e *Engine) buildDebrief() []string {
	blueHealth := int(math.Round(teamHealthRatio(e.session.Units, "blue") * 100))
	redHealth := int(math.Round(teamHealthRatio(e.session.Units, "red") * 100))
	items := []string{
		e.session.OutcomeReason,
		fmt.Sprintf("Combined remaining health: blue %d%%, red %d%%.", blueHealth, redHealth),
	}
	if objective := e.session.Objective; objective != nil {
		items = append(items, fmt.Sprintf("%s control ended blue %d/%d and red %d/%d.", objective.Label,
			objective.BlueProgress, objective.RequiredProgress, objective.RedProgress, objective.RequiredProgress))
	}
	if e.moment.Rules.Victory.AllowEscape {
		items = append(items, fmt.Sprintf("Escape progress ended at %d/%d turns.", e.session.EscapeProgress, e.moment.Rules.Victory.EscapeTurns))
	}
	items = append(items, "Scenario advantage is a rules-based state indicator, not a calibrated win probability.")
	return items
}

func (e *Engine) revealReferenceForTurn() {
	index := e.session.Turn - 1
	if index < 0 || index >= len(e.moment.Rules.ReferencePlan) {
		return
	}
	action := cloneAction(e.moment.Rules.ReferencePlan[index])
	e.session.LastReferenceAction = &action
	if index < len(e.moment.Rules.ReferenceReasons) {
		e.session.ReferenceReason = e.moment.Rules.ReferenceReasons[index]
	} else {
		e.session.ReferenceReason = "Authored tactical baseline for this scenario state."
	}
}

func (e *Engine) computeReferenceOutcomes() []model.ReferenceOutcome {
	outcomes := make([]model.ReferenceOutcome, 0, len(actionTypes))
	for _, actionType := range actionTypes {
		probe := newEngine(e.moment, "reference-"+actionType, false)
		firstAction := cloneAction(e.moment.Rules.ActionDefaults[actionType])
		_, err := probe.Apply(firstAction)
		continuation := e.moment.Rules.ReferenceContinuations[actionType]
		for err == nil && probe.session.Status == "active" && probe.session.Turn < probe.session.MaxTurns {
			index := probe.session.Turn - 1
			next := model.Action{Type: "hold"}
			if index >= 0 && index < len(continuation) {
				next = cloneAction(continuation[index])
			}
			if next.Type == "move" && next.Target == nil {
				next = cloneAction(e.moment.Rules.ActionDefaults["move"])
			}
			_, err = probe.Apply(next)
		}
		outcomes = append(outcomes, model.ReferenceOutcome{
			FirstAction: firstAction, Status: probe.session.Status, Turns: probe.session.Turn,
			Advantage: probe.session.Advantage, OutcomeReason: probe.session.OutcomeReason,
			KeyEvents: keyEvents(probe.session.Log, 3),
		})
	}
	return outcomes
}

type bestRolloutResult struct {
	actions []model.Action
	state   model.Session
}

func (e *Engine) computeBestCase() *model.BestCaseLine {
	best := bestContinuation(e.moment, nil)
	probe := newEngine(e.moment, "best-case-line", false)
	prefix := make([]model.Action, 0, len(best.actions))
	steps := make([]model.BestCaseStep, 0, len(best.actions))

	for _, action := range best.actions {
		before := probe.session.Advantage
		logStart := len(probe.session.Log)
		alternatives := bestCaseAlternatives(e.moment, prefix)
		_, err := probe.Apply(action)
		if err != nil {
			break
		}
		steps = append(steps, model.BestCaseStep{
			Turn:            probe.session.Turn,
			Action:          cloneAction(action),
			Reason:          bestCaseReason(action, probe.session, alternatives, e.moment),
			AdvantageBefore: before,
			AdvantageAfter:  probe.session.Advantage,
			KeyEvents:       stepEvents(probe.session.Log[logStart:], 4),
			Alternatives:    alternatives,
		})
		prefix = append(prefix, cloneAction(action))
		if probe.session.Status != "active" {
			break
		}
	}

	return &model.BestCaseLine{
		Status: probe.session.Status, Turns: probe.session.Turn, Advantage: probe.session.Advantage,
		OutcomeReason: probe.session.OutcomeReason,
		Method:        "Exhaustive deterministic search over all four modeled commands at every remaining turn; Move uses the scenario's authored destination.",
		Steps:         steps,
	}
}

func bestContinuation(moment model.Moment, prefix []model.Action) bestRolloutResult {
	probe := newEngine(moment, "best-case-probe", false)
	for _, action := range prefix {
		if probe.session.Status != "active" {
			break
		}
		_, _ = probe.Apply(action)
	}
	if probe.session.Status != "active" || len(prefix) >= moment.MaxTurns {
		return bestRolloutResult{actions: cloneActions(prefix), state: probe.session}
	}

	var best *bestRolloutResult
	for _, actionType := range actionTypes {
		action := cloneAction(moment.Rules.ActionDefaults[actionType])
		next := append(cloneActions(prefix), action)
		candidate := bestContinuation(moment, next)
		if best == nil || rolloutBetter(candidate, *best) {
			copy := candidate
			best = &copy
		}
	}
	return *best
}

func bestCaseAlternatives(moment model.Moment, prefix []model.Action) []model.BestCaseAlternative {
	alternatives := make([]model.BestCaseAlternative, 0, len(actionTypes))
	for _, actionType := range actionTypes {
		action := cloneAction(moment.Rules.ActionDefaults[actionType])
		result := bestContinuation(moment, append(cloneActions(prefix), action))
		alternatives = append(alternatives, model.BestCaseAlternative{
			Action: action, Status: result.state.Status, Turns: result.state.Turn,
			Advantage: result.state.Advantage, OutcomeReason: result.state.OutcomeReason,
		})
	}
	return alternatives
}

func rolloutBetter(candidate, current bestRolloutResult) bool {
	candidateRank, currentRank := statusRank(candidate.state.Status), statusRank(current.state.Status)
	if candidateRank != currentRank {
		return candidateRank > currentRank
	}
	if math.Abs(candidate.state.Advantage-current.state.Advantage) > .0001 {
		return candidate.state.Advantage > current.state.Advantage
	}
	candidateBlue, currentBlue := teamHealthRatio(candidate.state.Units, "blue"), teamHealthRatio(current.state.Units, "blue")
	if math.Abs(candidateBlue-currentBlue) > .0001 {
		return candidateBlue > currentBlue
	}
	candidateRed, currentRed := teamHealthRatio(candidate.state.Units, "red"), teamHealthRatio(current.state.Units, "red")
	if math.Abs(candidateRed-currentRed) > .0001 {
		return candidateRed < currentRed
	}
	if candidate.state.Status == "won" {
		return candidate.state.Turn < current.state.Turn
	}
	return candidate.state.Turn > current.state.Turn
}

func statusRank(status string) int {
	switch status {
	case "won":
		return 2
	case "active":
		return 1
	default:
		return 0
	}
}

func bestCaseReason(action model.Action, after model.Session, alternatives []model.BestCaseAlternative, moment model.Moment) string {
	tactical := tacticalActionReason(action, after, moment)
	var chosen model.BestCaseAlternative
	var nextBest *model.BestCaseAlternative
	tied := 0
	for _, alternative := range alternatives {
		if alternative.Action.Type == action.Type {
			chosen = alternative
			continue
		}
		if nextBest == nil || alternativeBetter(alternative, *nextBest) {
			copy := alternative
			nextBest = &copy
		}
	}
	for _, alternative := range alternatives {
		if alternative.Status == chosen.Status && alternative.Turns == chosen.Turns && math.Abs(alternative.Advantage-chosen.Advantage) <= .005 {
			tied++
		}
	}

	comparison := fmt.Sprintf("The search projected a %s in %d turns with a %.0f/100 rules-based advantage.",
		outcomeNoun(chosen.Status), chosen.Turns, chosen.Advantage*100)
	if tied > 1 {
		otherCount := tied - 1
		choiceLabel := "choice"
		if otherCount != 1 {
			choiceLabel = "choices"
		}
		comparison = fmt.Sprintf("This tied with %d other modeled %s for the strongest reachable result: a %s in %d turns at %.0f/100 rules-based advantage.",
			otherCount, choiceLabel, outcomeNoun(chosen.Status), chosen.Turns, chosen.Advantage*100)
	} else if nextBest != nil {
		comparison += fmt.Sprintf(" The next-best command projected a %s in %d turns at %.0f/100.",
			outcomeNoun(nextBest.Status), nextBest.Turns, nextBest.Advantage*100)
	}
	return tactical + " " + comparison
}

func outcomeNoun(status string) string {
	if status == "won" {
		return "win"
	}
	return "loss"
}

func tacticalActionReason(action model.Action, after model.Session, moment model.Moment) string {
	switch action.Type {
	case "move":
		if action.Target != nil {
			return fmt.Sprintf("Moving toward (%.0f, %.0f) used the scenario's authored repositioning lane before allies and enemies resolved their policies.", action.Target.X, action.Target.Y)
		}
		return "Moving used the scenario's authored repositioning lane before the next exchange."
	case "hold":
		return "Holding added 4 shield and reduced incoming damage by 35% while allied policies continued to resolve."
	case "contest":
		if after.VisibleEnemyCount > 0 {
			return "Contesting closed on the nearest visible threat and converted current vision into an attack whenever range allowed."
		}
		if moment.Rules.Objective != nil {
			return "With no visible enemy target, contesting advanced directly toward the modeled objective."
		}
		return "Contesting applied pressure to the nearest modeled threat."
	case "retreat":
		return "Retreating applied guard and moved 20% faster toward the authored safe zone, creating the strongest modeled disengage."
	default:
		return "This command produced the strongest modeled continuation from the current state."
	}
}

func alternativeBetter(candidate, current model.BestCaseAlternative) bool {
	candidateRank, currentRank := statusRank(candidate.Status), statusRank(current.Status)
	if candidateRank != currentRank {
		return candidateRank > currentRank
	}
	if math.Abs(candidate.Advantage-current.Advantage) > .0001 {
		return candidate.Advantage > current.Advantage
	}
	if candidate.Status == "won" {
		return candidate.Turns < current.Turns
	}
	return candidate.Turns > current.Turns
}

func stepEvents(log []model.LogEntry, limit int) []string {
	events := make([]string, 0, limit)
	for _, entry := range log {
		if entry.Actor != "user" && entry.Kind != "damage" && entry.Kind != "support" && entry.Kind != "objective" &&
			entry.Kind != "escape" && entry.Kind != "elimination" && entry.Kind != "outcome" {
			continue
		}
		events = append(events, entry.Message)
		if len(events) == limit {
			break
		}
	}
	return events
}

func (e *Engine) applyFog(logReveals bool) {
	visibleCount, unknownCount := 0, 0
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		previous := unit.Visible
		if unit.Team == "blue" {
			unit.Visible = true
			continue
		}
		unit.Visible = false
		if unit.Alive {
			for _, observer := range e.session.Units {
				if observer.Team == "blue" && observer.Alive && e.hasVision(observer, *unit) {
					unit.Visible = true
					break
				}
			}
		}
		if unit.Alive && unit.Visible {
			visibleCount++
		} else if unit.Alive {
			unknownCount++
		}
		if logReveals && !previous && unit.Visible {
			e.addLog("system", "vision", "revealed", unit.ID, "", 0,
				fmt.Sprintf("Vision revealed %s.", unitName(*unit)))
		}
	}
	e.session.VisibleEnemyCount = visibleCount
	e.session.UnknownEnemyCount = unknownCount
	e.session.VisionLimited = unknownCount > 0
}

func (e *Engine) hasVision(observer, target model.Unit) bool {
	d := distance(observer.Position, target.Position)
	if d > observer.VisionRange {
		return false
	}
	if d <= 12 {
		return true
	}
	for _, terrain := range e.moment.Rules.Terrain {
		if terrain.BlocksVision && distance(target.Position, terrain.Position) <= terrain.Radius {
			return false
		}
	}
	return true
}

func (e *Engine) enemyTarget(observer model.Unit) *model.Unit {
	var target *model.Unit
	best := math.MaxFloat64
	for i := range e.session.Units {
		candidate := &e.session.Units[i]
		if candidate.Team != "blue" || !candidate.Alive || !e.hasVision(observer, *candidate) {
			continue
		}
		d := distance(observer.Position, candidate.Position)
		priority := d
		if candidate.ID == e.moment.ControlledUnitID && observer.Policy == "aggressive" {
			priority -= 6
		}
		if priority < best {
			target, best = candidate, priority
		}
	}
	return target
}

func (e *Engine) nearestVisibleEnemy(from model.Unit) *model.Unit {
	var nearest *model.Unit
	best := math.MaxFloat64
	for i := range e.session.Units {
		candidate := &e.session.Units[i]
		if candidate.Alive && candidate.Team != from.Team && candidate.Visible {
			d := distance(from.Position, candidate.Position)
			if d < best {
				nearest, best = candidate, d
			}
		}
	}
	return nearest
}

func (e *Engine) movementMultiplier(position model.Point) float64 {
	multiplier := 1.0
	for _, terrain := range e.moment.Rules.Terrain {
		if terrain.MoveMultiplier > 0 && distance(position, terrain.Position) <= terrain.Radius {
			multiplier = math.Min(multiplier, terrain.MoveMultiplier)
		}
	}
	return multiplier
}

func (e *Engine) unit(id string) *model.Unit {
	for i := range e.session.Units {
		if e.session.Units[i].ID == id {
			return &e.session.Units[i]
		}
	}
	return nil
}

func (e *Engine) addLog(actor, kind, action, actorID, targetID string, value int, message string) {
	e.session.Log = append(e.session.Log, model.LogEntry{
		Turn: e.session.Turn, Actor: actor, Kind: kind, Action: action,
		ActorID: actorID, TargetID: targetID, Value: value, Message: message,
	})
}

func normalizeMoment(moment model.Moment) model.Moment {
	if moment.Rules.InitialAdvantage <= 0 || moment.Rules.InitialAdvantage >= 1 {
		moment.Rules.InitialAdvantage = .5
	}
	if moment.Rules.Victory.Kind == "" {
		moment.Rules.Victory.Kind = "skirmish"
	}
	if moment.Rules.Victory.Description == "" {
		moment.Rules.Victory.Description = "Finish the decision window with the stronger tactical state."
	}
	if moment.Rules.Victory.DefeatDescription == "" {
		moment.Rules.Victory.DefeatDescription = "The controlled unit was eliminated."
	}
	if moment.Rules.Victory.SafeRadius <= 0 {
		moment.Rules.Victory.SafeRadius = 8
	}
	if moment.Rules.Victory.EscapeTurns <= 0 {
		moment.Rules.Victory.EscapeTurns = 2
	}
	for len(moment.Rules.ReferencePlan) < moment.MaxTurns {
		moment.Rules.ReferencePlan = append(moment.Rules.ReferencePlan, model.Action{Type: "hold"})
	}
	if moment.Rules.ActionDefaults == nil {
		moment.Rules.ActionDefaults = map[string]model.Action{}
	}
	if moment.Rules.ReferenceContinuations == nil {
		moment.Rules.ReferenceContinuations = map[string][]model.Action{}
	}
	for _, actionType := range actionTypes {
		if _, ok := moment.Rules.ActionDefaults[actionType]; !ok {
			action := model.Action{Type: actionType}
			if actionType == "move" {
				target := model.Point{X: 50, Y: 50}
				action.Target = &target
			}
			moment.Rules.ActionDefaults[actionType] = action
		}
		if _, ok := moment.Rules.ReferenceContinuations[actionType]; !ok {
			continuation := make([]model.Action, max(0, moment.MaxTurns-1))
			for i := range continuation {
				planIndex := min(i+1, len(moment.Rules.ReferencePlan)-1)
				if planIndex >= 0 {
					continuation[i] = cloneAction(moment.Rules.ReferencePlan[planIndex])
				} else {
					continuation[i] = model.Action{Type: "hold"}
				}
			}
			moment.Rules.ReferenceContinuations[actionType] = continuation
		}
	}
	for i := range moment.Units {
		unit := &moment.Units[i]
		if unit.MaxHP <= 0 {
			unit.MaxHP = 100
		}
		if unit.AttackRange <= 0 {
			if unit.Role == "carry" || unit.Role == "mage" || unit.Role == "support" {
				unit.AttackRange = 22
			} else {
				unit.AttackRange = 9
			}
		}
		if unit.AttackDamage <= 0 {
			unit.AttackDamage = 16
		}
		if unit.MoveSpeed <= 0 {
			unit.MoveSpeed = 14
		}
		if unit.VisionRange <= 0 {
			unit.VisionRange = 34
		}
		if unit.AttackCooldown <= 0 {
			unit.AttackCooldown = 2
		}
		if unit.Policy == "" {
			switch unit.Role {
			case "support":
				unit.Policy = "support"
			case "tank":
				unit.Policy = "protector"
			case "mage":
				unit.Policy = "skirmisher"
			default:
				unit.Policy = "aggressive"
			}
		}
	}
	return moment
}

func pointOnMap(point model.Point) bool {
	return point.X >= 0 && point.X <= 100 && point.Y >= 0 && point.Y <= 100
}

func clampPoint(point model.Point) model.Point {
	return model.Point{X: clamp(point.X, 0, 100), Y: clamp(point.Y, 0, 100)}
}

func distance(a, b model.Point) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func moveToward(from, to model.Point, limit float64) model.Point {
	d := distance(from, to)
	if d == 0 || d <= limit {
		return to
	}
	return model.Point{X: from.X + (to.X-from.X)*limit/d, Y: from.Y + (to.Y-from.Y)*limit/d}
}

func moveAway(from, threat model.Point, limit float64) model.Point {
	d := distance(from, threat)
	if d == 0 {
		return model.Point{X: from.X + limit, Y: from.Y}
	}
	return clampPoint(model.Point{X: from.X + (from.X-threat.X)*limit/d, Y: from.Y + (from.Y-threat.Y)*limit/d})
}

func unitName(unit model.Unit) string {
	return strings.Title(unit.Team) + " " + unit.Role
}

func unitRole(id string, units []model.Unit) string {
	for _, unit := range units {
		if unit.ID == id {
			return unit.Role
		}
	}
	return "target"
}

func allTeamEliminated(units []model.Unit, team string) bool {
	found := false
	for _, unit := range units {
		if unit.Team == team {
			found = true
			if unit.Alive {
				return false
			}
		}
	}
	return found
}

func teamHealthRatio(units []model.Unit, team string) float64 {
	current, total := 0, 0
	for _, unit := range units {
		if unit.Team == team {
			current += unit.HP
			total += unit.MaxHP
		}
	}
	if total == 0 {
		return 0
	}
	return float64(current) / float64(total)
}

func teamAlive(units []model.Unit, team string) (int, int) {
	alive, total := 0, 0
	for _, unit := range units {
		if unit.Team == team {
			total++
			if unit.Alive {
				alive++
			}
		}
	}
	return alive, total
}

func keyEvents(log []model.LogEntry, limit int) []string {
	items := make([]string, 0, limit)
	for _, entry := range log {
		if entry.Kind != "damage" && entry.Kind != "objective" && entry.Kind != "escape" && entry.Kind != "outcome" {
			continue
		}
		items = append(items, entry.Message)
		if len(items) == limit {
			break
		}
	}
	return items
}

func cloneMoment(moment model.Moment) model.Moment {
	moment.Units = cloneUnits(moment.Units)
	moment.ReasonTags = slices.Clone(moment.ReasonTags)
	moment.Rules.Terrain = slices.Clone(moment.Rules.Terrain)
	moment.Rules.ReferenceReasons = slices.Clone(moment.Rules.ReferenceReasons)
	referencePlan := moment.Rules.ReferencePlan
	moment.Rules.ReferencePlan = make([]model.Action, len(referencePlan))
	for i, action := range referencePlan {
		moment.Rules.ReferencePlan[i] = cloneAction(action)
	}
	if moment.Rules.Objective != nil {
		objective := *moment.Rules.Objective
		moment.Rules.Objective = &objective
	}
	if moment.Rules.ActionDefaults != nil {
		actionDefaults := moment.Rules.ActionDefaults
		moment.Rules.ActionDefaults = make(map[string]model.Action, len(actionDefaults))
		for key, action := range actionDefaults {
			moment.Rules.ActionDefaults[key] = cloneAction(action)
		}
	}
	if moment.Rules.ReferenceContinuations != nil {
		continuations := moment.Rules.ReferenceContinuations
		moment.Rules.ReferenceContinuations = make(map[string][]model.Action, len(continuations))
		for key, actions := range continuations {
			cloned := make([]model.Action, len(actions))
			for i, action := range actions {
				cloned[i] = cloneAction(action)
			}
			moment.Rules.ReferenceContinuations[key] = cloned
		}
	}
	return moment
}

func cloneUnits(units []model.Unit) []model.Unit {
	return slices.Clone(units)
}

func cloneAction(action model.Action) model.Action {
	if action.Target != nil {
		target := *action.Target
		action.Target = &target
	}
	return action
}

func cloneReferenceOutcomes(outcomes []model.ReferenceOutcome) []model.ReferenceOutcome {
	cloned := make([]model.ReferenceOutcome, len(outcomes))
	for i, outcome := range outcomes {
		cloned[i] = outcome
		cloned[i].FirstAction = cloneAction(outcome.FirstAction)
		cloned[i].KeyEvents = slices.Clone(outcome.KeyEvents)
	}
	return cloned
}

func cloneBestCase(bestCase *model.BestCaseLine) *model.BestCaseLine {
	if bestCase == nil {
		return nil
	}
	cloned := *bestCase
	cloned.Steps = make([]model.BestCaseStep, len(bestCase.Steps))
	for i, step := range bestCase.Steps {
		cloned.Steps[i] = step
		cloned.Steps[i].Action = cloneAction(step.Action)
		cloned.Steps[i].KeyEvents = slices.Clone(step.KeyEvents)
		cloned.Steps[i].Alternatives = make([]model.BestCaseAlternative, len(step.Alternatives))
		for j, alternative := range step.Alternatives {
			cloned.Steps[i].Alternatives[j] = alternative
			cloned.Steps[i].Alternatives[j].Action = cloneAction(alternative.Action)
		}
	}
	return &cloned
}

func cloneActions(actions []model.Action) []model.Action {
	cloned := make([]model.Action, len(actions))
	for i, action := range actions {
		cloned[i] = cloneAction(action)
	}
	return cloned
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
