package fixtures

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
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
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode fixtures: %w", err)
	}
	if file.Version != "2.0" {
		return nil, fmt.Errorf("unsupported fixture version %q", file.Version)
	}
	if len(file.Moments) == 0 {
		return nil, fmt.Errorf("fixtures contain no moments")
	}
	seen := map[string]bool{}
	for _, moment := range file.Moments {
		if moment.ID == "" || moment.Slug == "" || moment.ControlledUnitID == "" {
			return nil, fmt.Errorf("moment identity fields cannot be empty")
		}
		if seen[moment.ID] {
			return nil, fmt.Errorf("duplicate moment id %q", moment.ID)
		}
		seen[moment.ID] = true
		if moment.MaxTurns < 1 || len(moment.Units) < 2 {
			return nil, fmt.Errorf("moment %q is not playable", moment.ID)
		}
		controlledFound := false
		unitIDs := map[string]bool{}
		for _, unit := range moment.Units {
			if unit.ID == "" || unitIDs[unit.ID] {
				return nil, fmt.Errorf("moment %q has an empty or duplicate unit id", moment.ID)
			}
			unitIDs[unit.ID] = true
			controlledFound = controlledFound || unit.ID == moment.ControlledUnitID
			if unit.MaxHP < 1 || unit.HP < 0 || unit.HP > unit.MaxHP || unit.AttackRange <= 0 ||
				unit.AttackDamage <= 0 || unit.MoveSpeed <= 0 || unit.VisionRange <= 0 || unit.AttackCooldown < 1 {
				return nil, fmt.Errorf("moment %q has invalid combat stats for unit %q", moment.ID, unit.ID)
			}
		}
		if !controlledFound {
			return nil, fmt.Errorf("moment %q does not contain controlled unit %q", moment.ID, moment.ControlledUnitID)
		}
		if moment.Rules.Victory.Description == "" || moment.Rules.Victory.DefeatDescription == "" {
			return nil, fmt.Errorf("moment %q must define explicit victory and defeat conditions", moment.ID)
		}
		if len(moment.Rules.ReferencePlan) < moment.MaxTurns {
			return nil, fmt.Errorf("moment %q reference plan must cover every turn", moment.ID)
		}
		for _, actionType := range []string{"move", "hold", "contest", "retreat"} {
			action, ok := moment.Rules.ActionDefaults[actionType]
			if !ok || action.Type != actionType || (actionType == "move" && action.Target == nil) {
				return nil, fmt.Errorf("moment %q must define a valid %q action default", moment.ID, actionType)
			}
			continuation, ok := moment.Rules.ReferenceContinuations[actionType]
			if !ok || len(continuation) < moment.MaxTurns-1 {
				return nil, fmt.Errorf("moment %q must define a full %q reference continuation", moment.ID, actionType)
			}
		}
		if objective := moment.Rules.Objective; objective != nil &&
			(objective.ID == "" || objective.Radius <= 0 || objective.CaptureTurns < 1) {
			return nil, fmt.Errorf("moment %q has invalid objective rules", moment.ID)
		}
	}
	return file.Moments, nil
}
