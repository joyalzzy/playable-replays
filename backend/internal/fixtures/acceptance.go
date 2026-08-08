package fixtures

import (
	"fmt"
	"strings"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

type AcceptanceResult struct {
	MomentID string `json:"momentId"`
	TestName string `json:"testName"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
}

func RunAcceptanceTests(moments []model.Moment) []AcceptanceResult {
	results := make([]AcceptanceResult, 0)
	for _, moment := range moments {
		for _, test := range moment.Authoring.AcceptanceTests {
			instance := engine.New(moment, "acceptance-"+moment.ID)
			state := instance.State()
			var applyErr error
			for actionIndex, action := range test.Actions {
				turnNumber := actionIndex + 1
				if containsTurn(test.DodgeBeforeTurns, turnNumber) {
					_, applyErr = instance.Dodge()
					if applyErr != nil {
						break
					}
				}
				state, applyErr = instance.Apply(action)
				if applyErr != nil || state.Status != "active" {
					break
				}
			}
			turnMatches := test.ExpectedTerminalTurn == 0 || state.Turn == test.ExpectedTerminalTurn
			passed := applyErr == nil && state.Status == test.ExpectedStatus && turnMatches &&
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

func containsTurn(turns []int, turn int) bool {
	for _, candidate := range turns {
		if candidate == turn {
			return true
		}
	}
	return false
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
