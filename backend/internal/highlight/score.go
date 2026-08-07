package highlight

import (
	"math"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

// Score applies the canonical interpretable highlight formula.
func Score(signals model.Signals) float64 {
	score := signals.WinProbabilitySwing*0.45 +
		signals.EventDensity*0.20 +
		(1-signals.EntityProximity)*0.20 +
		signals.ResourceAsymmetry*0.15
	return max(0, min(1, score))
}

// RoundedScore matches the detector's version 1.0 four-decimal output.
func RoundedScore(signals model.Signals) float64 {
	return math.Round(Score(signals)*10000) / 10000
}
