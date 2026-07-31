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
	}
	return file.Moments, nil
}
