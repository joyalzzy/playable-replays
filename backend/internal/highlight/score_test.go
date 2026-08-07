package highlight

import (
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestCanonicalScoreAndDetectorRounding(t *testing.T) {
	signals := model.Signals{
		WinProbabilitySwing: 0.88,
		EventDensity:        0.81,
		EntityProximity:     0.18,
		ResourceAsymmetry:   0.72,
	}
	if got := RoundedScore(signals); got != 0.83 {
		t.Fatalf("RoundedScore() = %v, want 0.83", got)
	}
	if got := Score(model.Signals{EntityProximity: 2}); got != 0 {
		t.Fatalf("Score() lower clamp = %v, want 0", got)
	}
	if got := Score(model.Signals{WinProbabilitySwing: 2, EventDensity: 2, ResourceAsymmetry: 2}); got != 1 {
		t.Fatalf("Score() upper clamp = %v, want 1", got)
	}
}
