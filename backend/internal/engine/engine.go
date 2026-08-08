package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

var ErrIllegalAction = errors.New("illegal action")
var ErrDodgeUnavailable = errors.New("dodge unavailable")
var ErrProjectileUnavailable = errors.New("projectile unavailable")

var actionTypes = []string{"move", "hold", "contest", "retreat"}

const teamHealthOutcomeMultiplier = 2
const teamHealthScenarioGoal = "Reach at least twice the opposing team's total remaining health before they reach twice yours."

type Engine struct {
	moment            model.Moment
	session           model.Session
	rng               *rand.Rand
	referenceOutcomes []model.ReferenceOutcome
	bestCase          *model.BestCaseLine
	botModel          BotModel
	rollouts          []BotRolloutRecord
	nextProjectileID  uint64
}

func New(moment model.Moment, sessionID string) *Engine {
	return newEngine(moment, sessionID, true, nil)
}

func NewWithBotModel(moment model.Moment, sessionID string, botModel BotModel) *Engine {
	return newEngine(moment, sessionID, true, botModel)
}

func newEngine(moment model.Moment, sessionID string, withReferences bool, botModel BotModel) *Engine {
	e := &Engine{moment: normalizeMoment(cloneMoment(moment)), botModel: botModel}
	e.Reset(sessionID)
	if withReferences {
		e.referenceOutcomes = e.computeReferenceOutcomes()
		e.bestCase = e.computeBestCase()
	}
	return e
}

func (e *Engine) Reset(sessionID string) model.Session {
	e.rng = rand.New(rand.NewSource(e.moment.Seed))
	e.rollouts = nil
	e.nextProjectileID = 0
	botSource := "deterministic-fallback"
	if e.botModel != nil {
		botSource = "pending"
	}
	e.session = model.Session{
		ID:                  sessionID,
		MomentID:            e.moment.ID,
		ControlledUnitID:    e.moment.ControlledUnitID,
		ScenarioGoal:        teamHealthScenarioGoal,
		MechanicBriefing:    cloneMechanicBriefing(e.moment.MechanicBriefing),
		MaxTurns:            e.moment.MaxTurns,
		Status:              "active",
		Advantage:           e.moment.Rules.InitialAdvantage,
		EscapeTurnsRequired: escapeTurnsRequired(e.moment.Rules.Victory),
		Terrain:             slices.Clone(e.moment.Rules.Terrain),
		Turrets:             canonicalTurrets(),
		Projectiles:         []model.Projectile{},
		ProjectileCharges:   2,
		DodgeCharges:        2,
		BotControl:          model.BotControlState{Source: botSource},
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
	e.updateDodgeAvailability()
	e.updateProjectileAvailability()
	e.recomputeAdvantage()
	return e.State()
}

func escapeTurnsRequired(victory model.VictoryRules) int {
	if !victory.AllowEscape {
		return 0
	}
	return victory.EscapeTurns
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
	state.Turrets = append([]model.Turret(nil), e.session.Turrets...)
	state.Projectiles = make([]model.Projectile, len(e.session.Projectiles))
	for index, projectile := range e.session.Projectiles {
		state.Projectiles[index] = projectile
		if projectile.Team == "red" {
			source := e.unit(projectile.SourceUnitID)
			if source == nil || !source.Visible {
				state.Projectiles[index].SourceUnitID = ""
			}
		}
	}
	state.MechanicBriefing = cloneMechanicBriefing(e.session.MechanicBriefing)
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
	return e.ApplyContext(context.Background(), action)
}

func (e *Engine) ApplyContext(ctx context.Context, action model.Action) (model.Session, error) {
	return e.ApplyTargetedContext(ctx, action, "")
}

func (e *Engine) ApplyTargetedContext(ctx context.Context, action model.Action, targetUnitID string) (model.Session, error) {
	if e.session.Status != "active" {
		return e.State(), fmt.Errorf("%w: session is complete", ErrIllegalAction)
	}
	controlled := e.unit(e.session.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return e.State(), fmt.Errorf("%w: controlled unit is unavailable", ErrIllegalAction)
	}
	if err := e.validatePlayerAction(action, targetUnitID); err != nil {
		return e.State(), err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	e.beginTurn()
	e.session.Turn++
	e.resolveProjectiles()
	if !controlled.Alive {
		e.recomputeAdvantage()
		e.evaluateOutcome()
		e.transferControlIfNeeded()
		e.revealReferenceForTurn()
		e.updateDodgeAvailability()
		e.updateProjectileAvailability()
		if e.session.Status != "active" {
			e.session.Debrief = e.buildDebrief()
		}
		return e.State(), nil
	}
	e.resolveUser(controlled, action, targetUnitID)
	e.applyFog(true)
	botActions, modelUsed, fallback := e.modelActions(ctx)
	e.resolveAllies(controlled, botActions, modelUsed)
	e.applyFog(true)
	e.resolveEnemies(controlled, botActions, modelUsed)
	e.logBotPolicy(modelUsed, fallback)
	e.applyFog(true)
	e.updateObjective()
	e.updateEscape()
	e.recomputeAdvantage()
	e.evaluateOutcome()
	e.transferControlIfNeeded()
	e.revealReferenceForTurn()
	e.updateDodgeAvailability()
	e.updateProjectileAvailability()
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

func (e *Engine) resolveUser(unit *model.Unit, action model.Action, targetUnitID string) {
	switch action.Type {
	case "move":
		e.moveUnit(unit, *action.Target, 1, "user", "reposition")
	case "hold":
		unit.Guarded = true
		unit.Shield = 4
		e.addLog("user", "defense", "hold", unit.ID, unit.ID, 4,
			fmt.Sprintf("%s held formation, gaining 4 shield and reducing incoming damage this turn.", unitName(*unit)))
	case "contest":
		if targetUnitID != "" {
			target := e.unit(targetUnitID)
			if target == nil || !target.Alive {
				e.addLog("user", "position", "contest", unit.ID, targetUnitID, 0,
					fmt.Sprintf("%s's selected target was no longer available when the attack resolved.", unitName(*unit)))
				return
			}
			if !e.performAttack(unit, target, "user", "contest") {
				e.addLog("user", "position", "cooldown", unit.ID, target.ID, 0,
					fmt.Sprintf("%s maintained pressure on %s while the attack cooldown recovered.", unitName(*unit), unitName(*target)))
			}
			return
		}
		target := e.controlledContestTarget(*unit)
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
			e.performAttack(unit, target, "user", "contest")
		} else {
			e.addLog("user", "position", "contest", unit.ID, target.ID, 0,
				fmt.Sprintf("%s closed distance but %s remained out of range.", unitName(*unit), unitName(*target)))
		}
	case "retreat":
		unit.Guarded = true
		destination := e.moment.Rules.Victory.SafeZone
		e.moveUnit(unit, destination, 1.2, "user", "retreat")
		e.addLog("user", "defense", "retreat", unit.ID, "", 0,
			fmt.Sprintf("%s disengaged toward the Blue base and reduced incoming damage this turn.", unitName(*unit)))
	}
}

func (e *Engine) resolveAllies(controlled *model.Unit, actions map[string]model.Action, modelUsed bool) {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.Team != "blue" || unit.ID == controlled.ID {
			continue
		}
		action, ok := actions[unit.ID]
		if !ok || !modelUsed {
			action = e.fallbackBotAction(*unit, controlled)
		}
		e.resolveBotAction(unit, action, "ally")
	}
}

func (e *Engine) resolveEnemies(controlled *model.Unit, actions map[string]model.Action, modelUsed bool) {
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		if !unit.Alive || unit.Team != "red" {
			continue
		}
		action, ok := actions[unit.ID]
		if !ok || !modelUsed {
			action = e.fallbackBotAction(*unit, controlled)
		}
		e.resolveBotAction(unit, action, "enemy")
	}
}

func (e *Engine) fallbackBotAction(unit model.Unit, controlled *model.Unit) model.Action {
	if unit.ID == e.moment.Rules.Victory.TargetUnitID {
		return model.Action{Type: "hold"}
	}
	if unit.MaxHP > 0 && float64(unit.HP)/float64(unit.MaxHP) < .35 {
		return model.Action{Type: "retreat"}
	}
	if unit.Policy == "support" || unit.Policy == "protector" {
		return model.Action{Type: "hold"}
	}
	if unit.Team == "blue" && e.nearestVisibleEnemy(unit) == nil {
		target := controlled.Position
		return model.Action{Type: "move", Target: &target}
	}
	return model.Action{Type: "contest"}
}

func (e *Engine) controlledContestTarget(unit model.Unit) *model.Unit {
	if targetID := e.moment.Rules.Victory.TargetUnitID; targetID != "" {
		target := e.unit(targetID)
		if target != nil && target.Alive && target.Team != unit.Team && target.Visible {
			return target
		}
	}
	return e.nearestVisibleEnemy(unit)
}

func (e *Engine) resolveBotAction(unit *model.Unit, action model.Action, actor string) {
	switch action.Type {
	case "move":
		if action.Target != nil {
			e.moveUnit(unit, *action.Target, 1, actor, "move")
		}
	case "hold":
		unit.Guarded = true
		unit.Shield += 4
		e.addLog(actor, "defense", "hold", unit.ID, unit.ID, 4,
			fmt.Sprintf("%s held position and gained 4 shield.", unitName(*unit)))
	case "retreat":
		unit.Guarded = true
		destination := model.Point{X: 8, Y: 92}
		if unit.Team == "red" {
			destination = model.Point{X: 92, Y: 8}
		}
		e.moveUnit(unit, destination, 1.2, actor, "retreat")
	case "contest":
		target := e.botTarget(*unit)
		if target == nil {
			if objective := e.moment.Rules.Objective; objective != nil {
				e.moveUnit(unit, objective.Position, .8, actor, "contest-objective")
			}
			return
		}
		if distance(unit.Position, target.Position) > unit.AttackRange {
			e.moveUnit(unit, target.Position, 1, actor, "contest")
		}
		if distance(unit.Position, target.Position) <= unit.AttackRange {
			e.performAttack(unit, target, actor, "contest")
		}
	}
}

func (e *Engine) botTarget(unit model.Unit) *model.Unit {
	if unit.Team == "red" {
		return e.enemyTarget(unit)
	}
	if unit.Policy == "aggressive" {
		controlled := e.unit(e.session.ControlledUnitID)
		target := e.unit(e.moment.Rules.Victory.TargetUnitID)
		if controlled != nil && controlled.Guarded && controlled.Shield > 0 && target != nil && target.Alive && target.Visible {
			return target
		}
	}
	return e.nearestVisibleEnemy(unit)
}

func (e *Engine) logBotPolicy(modelUsed, fallback bool) {
	if !modelUsed && !fallback {
		return
	}
	action := "model-actions"
	message := "The external model supplied every live bot action; the simulator validated and resolved them under authoritative rules."
	if modelUsed && len(e.rollouts) > 0 {
		record := e.rollouts[len(e.rollouts)-1]
		e.session.BotControl = model.BotControlState{
			Source: "external-model", ModelName: record.ModelName, ModelVersion: record.ModelVersion,
		}
	}
	if fallback {
		action = "fallback"
		message = "The external bot model was unavailable or unusable; deterministic bot actions were applied for this turn."
		e.session.BotControl = model.BotControlState{Source: "deterministic-fallback"}
	}
	e.addLog("policy", "model", action, "", "", 0, message)
}

func (e *Engine) moveUnit(unit *model.Unit, destination model.Point, speedScale float64, actor, action string) {
	if !unit.Alive {
		return
	}
	public := unit.Team == "blue" || unit.Visible
	start := unit.Position
	classLimit := unit.MoveRange
	if classLimit <= 0 {
		classLimit = unit.MoveSpeed
	}
	requestedLimit := unit.MoveSpeed * speedScale
	if requestedLimit <= 0 {
		requestedLimit = classLimit
	}
	limit := min(classLimit, requestedLimit) * e.movementMultiplier(start)
	unit.Position = clampPoint(moveToward(start, destination, limit))
	moved := distance(start, unit.Position)
	if moved < .05 || (actor == "enemy" && !public) {
		return
	}
	e.addLog(actor, "movement", action, unit.ID, "", 0,
		fmt.Sprintf("%s moved %.1f map units toward (%.0f, %.0f).", unitName(*unit), moved, destination.X, destination.Y))
}

func (e *Engine) performAttack(attacker, target *model.Unit, actor, action string) bool {
	if attacker.Class == model.ClassMarksman {
		return e.fireProjectile(attacker, target, actor)
	}
	return e.attack(attacker, target, actor, action)
}

func (e *Engine) fireProjectile(attacker, target *model.Unit, actor string) bool {
	if attacker == nil || target == nil || attacker.Class != model.ClassMarksman ||
		attacker.Team == target.Team || attacker.Cooldown > 0 || !attacker.Alive || !target.Alive ||
		distance(attacker.Position, target.Position) > attacker.AttackRange {
		return false
	}
	e.nextProjectileID++
	damage := max(1, (target.MaxHP+1)/2)
	projectile := model.Projectile{
		ID:   fmt.Sprintf("projectile-%d-%d", e.session.Turn, e.nextProjectileID),
		Team: attacker.Team, SourceUnitID: attacker.ID, TargetUnitID: target.ID,
		Position: attacker.Position, Target: target.Position, Damage: damage,
	}
	e.session.Projectiles = append(e.session.Projectiles, projectile)
	attacker.Cooldown = attacker.AttackCooldown
	sourceLabel := unitName(*attacker)
	actorID := attacker.ID
	if attacker.Team == "red" && !attacker.Visible {
		sourceLabel = "An unseen marksman"
		actorID = ""
	}
	e.addLog(actor, "projectile", "fired", actorID, target.ID, damage,
		fmt.Sprintf("%s fired a marksman projectile at %s for %d potential damage.", sourceLabel, unitName(*target), damage))
	return true
}

// FireProjectile spends one of the player's two projectile charges and queues
// a shot from a live blue marksman. It is a reaction and does not advance the
// tactical turn, model, objective, or reference search.
func (e *Engine) FireProjectile(sourceUnitID, targetUnitID string) (model.Session, error) {
	if e.session.Status != "active" || e.session.ProjectileCharges <= 0 {
		return e.State(), ErrProjectileUnavailable
	}
	source := e.unit(sourceUnitID)
	target := e.unit(targetUnitID)
	if !e.canFirePlayerProjectile(source, target) {
		return e.State(), ErrProjectileUnavailable
	}
	if !e.fireProjectile(source, target, "user") {
		return e.State(), ErrProjectileUnavailable
	}
	e.session.ProjectileCharges--
	e.updateDodgeAvailability()
	e.updateProjectileAvailability()
	return e.State(), nil
}

func (e *Engine) canFirePlayerProjectile(source, target *model.Unit) bool {
	return source != nil && target != nil && source.Team == "blue" &&
		source.Class == model.ClassMarksman && source.Alive && source.Cooldown == 0 &&
		e.isPlayerProjectileSource(source) &&
		target.Team == "red" && target.Alive && target.Visible &&
		distance(source.Position, target.Position) <= source.AttackRange
}

func (e *Engine) isPlayerProjectileSource(source *model.Unit) bool {
	controlled := e.unit(e.session.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return false
	}
	if controlled.Class == model.ClassMarksman {
		return source.ID == controlled.ID
	}
	return source.ID != controlled.ID
}

func (e *Engine) updateProjectileAvailability() {
	e.session.ProjectileAvailable = false
	if e.session.Status != "active" || e.session.ProjectileCharges <= 0 {
		return
	}
	for sourceIndex := range e.session.Units {
		source := &e.session.Units[sourceIndex]
		for targetIndex := range e.session.Units {
			if e.canFirePlayerProjectile(source, &e.session.Units[targetIndex]) {
				e.session.ProjectileAvailable = true
				return
			}
		}
	}
}

func (e *Engine) resolveProjectiles() {
	pending := append([]model.Projectile(nil), e.session.Projectiles...)
	e.session.Projectiles = e.session.Projectiles[:0]
	for _, projectile := range pending {
		target := e.unit(projectile.TargetUnitID)
		if target == nil || !target.Alive {
			continue
		}
		before := target.HP
		damage := min(projectile.Damage, target.HP)
		target.HP -= damage
		target.Alive = target.HP > 0
		sourceID := e.publicProjectileSourceID(projectile)
		e.addLog("system", "projectile", "hit", sourceID, target.ID, damage,
			fmt.Sprintf("The marksman projectile hit %s for %d damage (%d to %d HP).", unitName(*target), damage, before, target.HP))
		if !target.Alive {
			e.addLog("system", "elimination", "eliminated", sourceID, target.ID, 0,
				fmt.Sprintf("%s was eliminated by a marksman projectile.", unitName(*target)))
		}
	}
}

func (e *Engine) publicProjectileSourceID(projectile model.Projectile) string {
	if projectile.Team != "red" {
		return projectile.SourceUnitID
	}
	source := e.unit(projectile.SourceUnitID)
	if source == nil || !source.Visible {
		return ""
	}
	return projectile.SourceUnitID
}

// Dodge evades the currently incoming marksman projectile without consuming a
// tactical turn. It is a separate, two-charge reaction and is never part of
// LegalActions or the decision-tree search.
func (e *Engine) Dodge() (model.Session, error) {
	if e.session.Status != "active" || e.session.DodgeCharges <= 0 {
		return e.State(), ErrDodgeUnavailable
	}
	controlled := e.unit(e.session.ControlledUnitID)
	if controlled == nil || !controlled.Alive {
		return e.State(), ErrDodgeUnavailable
	}
	projectileIndex := -1
	for index, projectile := range e.session.Projectiles {
		if projectile.Team == "red" && projectile.TargetUnitID == controlled.ID {
			projectileIndex = index
			break
		}
	}
	if projectileIndex < 0 {
		return e.State(), ErrDodgeUnavailable
	}

	projectile := e.session.Projectiles[projectileIndex]
	e.session.Projectiles = append(e.session.Projectiles[:projectileIndex], e.session.Projectiles[projectileIndex+1:]...)
	e.session.DodgeCharges--
	destination := e.automaticDodgeTarget(*controlled)
	e.moveUnit(controlled, destination, 1, "user", "evade-projectile")
	e.addLog("user", "projectile", "evaded", controlled.ID, projectile.ID, projectile.Damage,
		fmt.Sprintf("%s evaded the incoming marksman projectile; %d Dodge charge%s remain.",
			unitName(*controlled), e.session.DodgeCharges, plural(e.session.DodgeCharges)))
	e.applyFog(true)
	e.updateDodgeAvailability()
	e.updateProjectileAvailability()
	return e.State(), nil
}

func (e *Engine) updateDodgeAvailability() {
	e.session.DodgeAvailable = false
	if e.session.Status != "active" || e.session.DodgeCharges <= 0 {
		return
	}
	for _, projectile := range e.session.Projectiles {
		if projectile.Team == "red" && projectile.TargetUnitID == e.session.ControlledUnitID {
			e.session.DodgeAvailable = true
			return
		}
	}
}

func plural(value int) string {
	if value == 1 {
		return ""
	}
	return "s"
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
		e.addLog("system", "elimination", "eliminated", actorID, target.ID, 0,
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
	controlled := e.unit(e.session.ControlledUnitID)
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
			"The Blue base is contested; escape progress reset.")
		return
	}
	e.session.EscapeProgress = min(rules.EscapeTurns, e.session.EscapeProgress+1)
	e.addLog("system", "escape", "progress", controlled.ID, "", e.session.EscapeProgress,
		fmt.Sprintf("Escape stabilized for %d/%d required turns.", e.session.EscapeProgress, rules.EscapeTurns))
}

func (e *Engine) evaluateOutcome() {
	controlled := e.unit(e.session.ControlledUnitID)
	playerTeam := "blue"
	if controlled != nil {
		playerTeam = controlled.Team
	}
	opponentTeam := opposingTeam(playerTeam)
	playerHealth := teamTotalHealth(e.session.Units, playerTeam)
	opponentHealth := teamTotalHealth(e.session.Units, opponentTeam)

	if playerHealth <= 0 {
		e.finish("lost", fmt.Sprintf("Your team has no remaining health; the opposing team has %d.", opponentHealth))
		return
	}
	if opponentHealth <= 0 || playerHealth >= teamHealthOutcomeMultiplier*opponentHealth {
		e.finish("won", fmt.Sprintf("Your team reached a %d:%d total-health lead (%d to %d).",
			teamHealthOutcomeMultiplier, 1, playerHealth, opponentHealth))
		return
	}
	if opponentHealth >= teamHealthOutcomeMultiplier*playerHealth {
		e.finish("lost", fmt.Sprintf("The opposing team reached a %d:%d total-health lead (%d to %d).",
			teamHealthOutcomeMultiplier, 1, opponentHealth, playerHealth))
	}
}

func (e *Engine) transferControlIfNeeded() {
	if e.session.Status != "active" {
		return
	}
	controlled := e.unit(e.session.ControlledUnitID)
	if controlled != nil && controlled.Alive {
		return
	}
	team := "blue"
	previousID := e.session.ControlledUnitID
	if controlled != nil {
		team = controlled.Team
	}
	for index := range e.session.Units {
		candidate := &e.session.Units[index]
		if candidate.Team != team || !candidate.Alive {
			continue
		}
		e.session.ControlledUnitID = candidate.ID
		e.addLog("system", "control", "transferred", previousID, candidate.ID, 0,
			fmt.Sprintf("Control transferred to %s so play can continue toward a decisive team-health result.", unitName(*candidate)))
		return
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
		fmt.Sprintf("Total remaining health: blue %d, red %d.", teamTotalHealth(e.session.Units, "blue"), teamTotalHealth(e.session.Units, "red")),
		fmt.Sprintf("Combined remaining health: blue %d%%, red %d%%.", blueHealth, redHealth),
	}
	if objective := e.session.Objective; objective != nil {
		items = append(items, fmt.Sprintf("%s control ended blue %d/%d and red %d/%d.", objective.Label,
			objective.BlueProgress, objective.RequiredProgress, objective.RedProgress, objective.RequiredProgress))
	}
	if e.moment.Rules.Victory.AllowEscape {
		items = append(items, fmt.Sprintf("Escape progress ended at %d/%d turns.", e.session.EscapeProgress, e.moment.Rules.Victory.EscapeTurns))
	}
	items = append(items, fmt.Sprintf("Dodge reactions remaining: %d of 2.", e.session.DodgeCharges))
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
		probe := newEngine(e.moment, "reference-"+actionType, false, nil)
		firstAction := cloneAction(e.moment.Rules.ActionDefaults[actionType])
		_, err := applyReferenceTurn(probe, firstAction)
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
			_, err = applyReferenceTurn(probe, next)
		}
		outcomes = append(outcomes, model.ReferenceOutcome{
			FirstAction: firstAction, Status: probe.session.Status, Turns: probe.session.Turn,
			Advantage: probe.session.Advantage, OutcomeReason: projectionOutcomeReason(probe.session),
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
	probe := newEngine(e.moment, "best-case-line", false, nil)
	prefix := make([]model.Action, 0, len(best.actions))
	steps := make([]model.BestCaseStep, 0, len(best.actions))

	for _, action := range best.actions {
		before := probe.session.Advantage
		logStart := len(probe.session.Log)
		alternatives := bestCaseAlternatives(e.moment, prefix)
		_, err := applyReferenceTurn(probe, action)
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
		OutcomeReason: projectionOutcomeReason(probe.session),
		Method:        "Exhaustive deterministic search over all four tactical commands within the authored teaching horizon; Move uses the scenario's authored destination and incoming projectiles use the same two-charge reference reaction.",
		Steps:         steps,
	}
}

func bestContinuation(moment model.Moment, prefix []model.Action) bestRolloutResult {
	probe := newEngine(moment, "best-case-probe", false, nil)
	for _, action := range prefix {
		if probe.session.Status != "active" {
			break
		}
		_, _ = applyReferenceTurn(probe, action)
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
			Advantage: result.state.Advantage, OutcomeReason: projectionOutcomeReason(result.state),
		})
	}
	return alternatives
}

func applyReferenceTurn(probe *Engine, action model.Action) (model.Session, error) {
	if probe.session.DodgeAvailable && probe.session.DodgeCharges > 0 {
		_, _ = probe.Dodge()
	}
	return probe.Apply(action)
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
	switch status {
	case "won":
		return "win"
	case "active":
		return "still-contested state"
	default:
		return "loss"
	}
}

func projectionOutcomeReason(state model.Session) string {
	if state.OutcomeReason != "" {
		return state.OutcomeReason
	}
	return fmt.Sprintf("Neither team reached the %d:%d total-health threshold within the authored %d-turn teaching horizon.",
		teamHealthOutcomeMultiplier, 1, state.MaxTurns)
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
			return "Contesting focused the visible authored target when present, otherwise the nearest visible threat, and converted current vision into an attack whenever range allowed."
		}
		if moment.Rules.Objective != nil {
			return "With no visible enemy target, contesting advanced directly toward the modeled objective."
		}
		return "Contesting applied pressure to the nearest modeled threat."
	case "retreat":
		return "Retreating applied guard and moved 20% faster toward the Blue base, creating the strongest modeled disengage."
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
		if entry.Actor != "user" && entry.Kind != "damage" && entry.Kind != "projectile" && entry.Kind != "support" && entry.Kind != "objective" &&
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
		if candidate.ID == e.session.ControlledUnitID && observer.Policy == "aggressive" {
			priority -= 14
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
		moment.Rules.Victory.Description = "Build a decisive team-health advantage."
	}
	if moment.Rules.Victory.DefeatDescription == "" {
		moment.Rules.Victory.DefeatDescription = "Avoid conceding a decisive team-health deficit."
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
		*unit = model.ApplyClassProfile(*unit)
		if unit.AttackDamage <= 0 {
			unit.AttackDamage = 16
		}
		unit.MoveSpeed = unit.MoveRange
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

func teamTotalHealth(units []model.Unit, team string) int {
	total := 0
	for _, unit := range units {
		if unit.Team == team {
			total += max(0, unit.HP)
		}
	}
	return total
}

func opposingTeam(team string) string {
	if team == "red" {
		return "blue"
	}
	return "red"
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
		if entry.Kind != "damage" && entry.Kind != "projectile" && entry.Kind != "objective" && entry.Kind != "escape" && entry.Kind != "outcome" {
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
	moment.MechanicBriefing = cloneMechanicBriefing(moment.MechanicBriefing)
	if moment.ReplayEvidence != nil {
		evidence := *moment.ReplayEvidence
		evidence.CaptionEvidence = slices.Clone(evidence.CaptionEvidence)
		evidence.ExternalEvidence = slices.Clone(evidence.ExternalEvidence)
		moment.ReplayEvidence = &evidence
	}
	moment.Authoring.IntendedTradeoffs = slices.Clone(moment.Authoring.IntendedTradeoffs)
	alternatives := moment.Authoring.PlausibleAlternatives
	moment.Authoring.PlausibleAlternatives = make([]model.ScenarioAlternative, len(alternatives))
	for i, alternative := range alternatives {
		moment.Authoring.PlausibleAlternatives[i] = alternative
		moment.Authoring.PlausibleAlternatives[i].Action = cloneAction(alternative.Action)
	}
	acceptanceTests := moment.Authoring.AcceptanceTests
	moment.Authoring.AcceptanceTests = make([]model.ScenarioAcceptanceTest, len(acceptanceTests))
	for i, acceptanceTest := range acceptanceTests {
		moment.Authoring.AcceptanceTests[i] = acceptanceTest
		moment.Authoring.AcceptanceTests[i].Actions = cloneActions(acceptanceTest.Actions)
		moment.Authoring.AcceptanceTests[i].DodgeBeforeTurns = slices.Clone(acceptanceTest.DodgeBeforeTurns)
	}
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

func cloneMechanicBriefing(briefing *model.MechanicBriefing) *model.MechanicBriefing {
	if briefing == nil {
		return nil
	}
	cloned := *briefing
	cloned.Mechanics = slices.Clone(briefing.Mechanics)
	return &cloned
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
