package fixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const maxUnitsPerMoment = 64

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
		return nil, fmt.Errorf("decode fixtures: expected one JSON value")
	}
	if file.Version != "1.0" {
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
		if len(moment.Units) > maxUnitsPerMoment {
			return nil, fmt.Errorf("moment %q exceeds the %d-unit simulation limit", moment.ID, maxUnitsPerMoment)
		}
		unitIDs := make(map[string]bool, len(moment.Units))
		controlledFound := false
		for _, unit := range moment.Units {
			if unit.ID == "" || unit.Role == "" {
				return nil, fmt.Errorf("moment %q contains a unit with empty identity fields", moment.ID)
			}
			if unitIDs[unit.ID] {
				return nil, fmt.Errorf("moment %q contains duplicate unit id %q", moment.ID, unit.ID)
			}
			unitIDs[unit.ID] = true
			if unit.ID == moment.ControlledUnitID {
				controlledFound = true
				if !unit.Alive {
					return nil, fmt.Errorf("moment %q controlled unit %q must be alive", moment.ID, unit.ID)
				}
			}
			if unit.Team != "blue" && unit.Team != "red" {
				return nil, fmt.Errorf("moment %q unit %q has invalid team %q", moment.ID, unit.ID, unit.Team)
			}
			profile, ok := model.Profile(unit.Class)
			if !ok {
				return nil, fmt.Errorf("moment %q unit %q has invalid class %q", moment.ID, unit.ID, unit.Class)
			}
			if unit.MaxHP != profile.MaxHP {
				return nil, fmt.Errorf(
					"moment %q unit %q maxHp must be %d for class %s",
					moment.ID, unit.ID, profile.MaxHP, unit.Class,
				)
			}
			if unit.HP < 0 || unit.HP > unit.MaxHP {
				return nil, fmt.Errorf("moment %q unit %q has invalid health", moment.ID, unit.ID)
			}
			if unit.Alive != (unit.HP > 0) {
				return nil, fmt.Errorf("moment %q unit %q has inconsistent alive state", moment.ID, unit.ID)
			}
			if unit.Cooldown < 0 {
				return nil, fmt.Errorf("moment %q unit %q has a negative cooldown", moment.ID, unit.ID)
			}
			if !finite(unit.Position.X) || !finite(unit.Position.Y) ||
				unit.Position.X < 0 || unit.Position.X > 100 ||
				unit.Position.Y < 0 || unit.Position.Y > 100 {
				return nil, fmt.Errorf("moment %q unit %q is outside the map", moment.ID, unit.ID)
			}
		}
		if !controlledFound {
			return nil, fmt.Errorf("moment %q controlled unit %q does not exist", moment.ID, moment.ControlledUnitID)
		}
	}
	return file.Moments, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
