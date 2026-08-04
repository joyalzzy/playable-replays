package engine

import (
	"slices"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func cloneMoment(moment model.Moment) model.Moment {
	moment.Units = cloneUnits(moment.Units)
	moment.ReasonTags = slices.Clone(moment.ReasonTags)
	return moment
}

func cloneUnits(units []model.Unit) []model.Unit {
	return slices.Clone(units)
}
