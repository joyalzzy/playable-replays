package fixtures

import (
	"fmt"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

type AcceptanceResult struct {
	MomentID string
	TestName string
	Passed   bool
	Detail   string
}

func RunAcceptanceTests(moments []model.Moment) []AcceptanceResult {
	results := make([]AcceptanceResult, 0)
	for _, moment := range moments {
		for _, test := range moment.Authoring.AcceptanceTests {
			instance := engine.New(moment, "acceptance-"+moment.ID)
			state := instance.State()
			var applyErr error
			for _, action := range test.Actions {
				state, applyErr = instance.Apply(action)
				if applyErr != nil || state.Status != "active" {
					break
				}
			}
			passed := applyErr == nil && state.Status == test.ExpectedStatus &&
				state.Turn == test.ExpectedTerminalTurn &&
				strings.Contains(strings.ToLower(state.OutcomeReason), strings.ToLower(test.ExpectedOutcomeContains))
			detail := fmt.Sprintf("expected %s on turn %d containing %q; got %s on turn %d: %q",
				test.ExpectedStatus, test.ExpectedTerminalTurn, test.ExpectedOutcomeContains,
				state.Status, state.Turn, state.OutcomeReason)
			if applyErr != nil {
				detail += "; apply error: " + applyErr.Error()
			}
			results = append(results, AcceptanceResult{
				MomentID: moment.ID,
				TestName: test.Name,
				Passed:   passed,
				Detail:   detail,
			})
		}
	}
	return results
}

func ValidateAcceptance(moments []model.Moment) error {
	failures := make([]string, 0)
	for _, result := range RunAcceptanceTests(moments) {
		if !result.Passed {
			failures = append(failures, fmt.Sprintf("%s/%s: %s", result.MomentID, result.TestName, result.Detail))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("fixture acceptance failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}
