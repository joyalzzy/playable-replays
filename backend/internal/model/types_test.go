package model

import (
	"encoding/json"
	"testing"
)

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

func TestSessionGameplayContractRoundTripsThroughJSON(t *testing.T) {
	session := Session{
		Turrets: []Turret{{
			ID: "blue-top-turret", Team: "blue", Lane: "top", Position: Point{X: 14, Y: 60},
			HP: 3000, MaxHP: 3000, Alive: true,
		}},
		Projectiles: []Projectile{{
			ID: "projectile-1", Team: "red", SourceUnitID: "red-marksman", TargetUnitID: "blue-carry",
			Position: Point{X: 60, Y: 40}, Target: Point{X: 30, Y: 50}, Damage: 45,
		}},
		ProjectileCharges: 2, ProjectileAvailable: true,
		DodgeCharges: 2, DodgeAvailable: true,
		BotControl: BotControlState{Source: "external-model", ModelName: "action-policy", ModelVersion: "2"},
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"turrets", "projectiles", "projectileCharges", "projectileAvailable", "dodgeCharges", "dodgeAvailable", "botControl"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("session JSON omitted %q: %s", name, encoded)
		}
	}
	var decoded Session
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Turrets) != 1 || decoded.Turrets[0].Lane != "top" ||
		len(decoded.Projectiles) != 1 || decoded.Projectiles[0].Damage != 45 ||
		decoded.ProjectileCharges != 2 || !decoded.ProjectileAvailable ||
		decoded.DodgeCharges != 2 || !decoded.DodgeAvailable ||
		decoded.BotControl.Source != "external-model" || decoded.BotControl.ModelName != "action-policy" {
		t.Fatalf("session gameplay fields changed during JSON round trip: %+v", decoded)
	}
}
