package fixtures

import "testing"

func TestVersionTwoFixturesContainAuthoredRules(t *testing.T) {
	moments, err := Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(moments) != 2 {
		t.Fatalf("expected two moments, got %d", len(moments))
	}
	for _, moment := range moments {
		if moment.Rules.Victory.Description == "" || len(moment.Rules.ReferencePlan) != moment.MaxTurns || len(moment.Rules.ActionDefaults) != 4 {
			t.Fatalf("moment %q is missing authored scenario rules", moment.ID)
		}
		for _, unit := range moment.Units {
			if unit.AttackRange <= 0 || unit.AttackDamage <= 0 || unit.MoveSpeed <= 0 || unit.VisionRange <= 0 {
				t.Fatalf("unit %q is missing explicit combat stats", unit.ID)
			}
		}
	}
}
