package model

import "testing"

func TestClassProfiles(t *testing.T) {
	tests := []struct {
		class       UnitClass
		maxHP       int
		moveRange   float64
		attackRange float64
	}{
		{ClassTank, 160, 7, 10},
		{ClassFighter, 125, 10, 14},
		{ClassMarksman, 90, 11, 28},
		{ClassMage, 95, 9, 24},
		{ClassSupport, 110, 8, 20},
		{ClassAssassin, 100, 13, 12},
	}
	for _, test := range tests {
		t.Run(string(test.class), func(t *testing.T) {
			profile, ok := Profile(test.class)
			if !ok {
				t.Fatal("profile was not found")
			}
			if profile.MaxHP != test.maxHP || profile.MoveRange != test.moveRange || profile.AttackRange != test.attackRange {
				t.Fatalf("unexpected profile: %+v", profile)
			}
		})
	}
}

func TestApplyClassProfileDerivesLegacyRoleAndPreservesHealthRatio(t *testing.T) {
	unit := ApplyClassProfile(Unit{Role: "carry", HP: 50, MaxHP: 100})
	if unit.Class != ClassMarksman {
		t.Fatalf("expected marksman, got %q", unit.Class)
	}
	if unit.HP != 45 || unit.MaxHP != 90 || unit.MoveRange != 11 || unit.AttackRange != 28 {
		t.Fatalf("unexpected normalized unit: %+v", unit)
	}
}

func TestExplicitClassOverridesLegacyRole(t *testing.T) {
	unit := ApplyClassProfile(Unit{Role: "carry", Class: ClassTank, HP: 160, MaxHP: 160})
	if unit.Class != ClassTank || unit.MaxHP != 160 || unit.MoveRange != 7 || unit.AttackRange != 10 {
		t.Fatalf("explicit class was not authoritative: %+v", unit)
	}
}
