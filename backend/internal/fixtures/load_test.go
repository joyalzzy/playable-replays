package fixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func validFixture() fileFormat {
	return fileFormat{
		Version: "1.0",
		Moments: []model.Moment{
			{
				ID: "m1", Slug: "moment-one", Title: "Moment", Description: "Test",
				Map: "Synthetic", Seed: 1, MaxTurns: 3, ControlledUnitID: "blue",
				ReasonTags: []string{"clutch"},
				Units: []model.Unit{
					{ID: "blue", Team: "blue", Role: "carry", Class: model.ClassMarksman, Position: model.Point{X: 30, Y: 50}, HP: 70, MaxHP: 90, Alive: true},
					{ID: "red", Team: "red", Role: "frontline", Class: model.ClassTank, Position: model.Point{X: 50, Y: 50}, HP: 120, MaxHP: 160, Alive: true},
				},
			},
		},
	}
}

func writeFixture(t *testing.T, fixture fileFormat) string {
	t.Helper()
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "moments.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidatesPlayableClassFixture(t *testing.T) {
	moments, err := Load(writeFixture(t, validFixture()))
	if err != nil {
		t.Fatal(err)
	}
	if len(moments) != 1 || moments[0].Units[0].Class != model.ClassMarksman {
		t.Fatalf("unexpected moments: %+v", moments)
	}
}

func TestLoadRejectsInvalidUnits(t *testing.T) {
	tests := map[string]func(*fileFormat){
		"unknown class":      func(file *fileFormat) { file.Moments[0].Units[0].Class = "wizard" },
		"class health":       func(file *fileFormat) { file.Moments[0].Units[0].MaxHP = 160 },
		"health overflow":    func(file *fileFormat) { file.Moments[0].Units[0].HP = 91 },
		"alive mismatch":     func(file *fileFormat) { file.Moments[0].Units[0].Alive = false },
		"invalid team":       func(file *fileFormat) { file.Moments[0].Units[0].Team = "green" },
		"negative cooldown":  func(file *fileFormat) { file.Moments[0].Units[0].Cooldown = -1 },
		"outside map":        func(file *fileFormat) { file.Moments[0].Units[0].Position.X = 101 },
		"duplicate id":       func(file *fileFormat) { file.Moments[0].Units[1].ID = "blue" },
		"missing controlled": func(file *fileFormat) { file.Moments[0].ControlledUnitID = "missing" },
		"empty role":         func(file *fileFormat) { file.Moments[0].Units[0].Role = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := validFixture()
			mutate(&fixture)
			if _, err := Load(writeFixture(t, fixture)); err == nil {
				t.Fatal("expected invalid fixture to be rejected")
			}
		})
	}
}

func TestLoadRejectsMoreThanSnapshotLimit(t *testing.T) {
	fixture := validFixture()
	fixture.Moments[0].Units = fixture.Moments[0].Units[:1]
	for i := 1; i < 65; i++ {
		fixture.Moments[0].Units = append(fixture.Moments[0].Units, model.Unit{
			ID: "red-" + strings.Repeat("x", i), Team: "red", Role: "fighter",
			Class: model.ClassFighter, Position: model.Point{X: 50, Y: 50},
			HP: 100, MaxHP: 125, Alive: true,
		})
	}
	if _, err := Load(writeFixture(t, fixture)); err == nil {
		t.Fatal("expected the 65-unit fixture to be rejected")
	}
}

func TestLoadRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for name, body := range map[string]string{
		"unknown":  `{"version":"1.0","moments":[],"admin":true}`,
		"trailing": `{"version":"1.0","moments":[]} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "moments.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid JSON fixture to be rejected")
			}
		})
	}
}
