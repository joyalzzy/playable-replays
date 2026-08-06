package main

import "testing"

func TestLoadPositionModelConfig(t *testing.T) {
	variables := []string{
		"POSITION_MODEL_URL", "POSITION_MODEL_NAME", "POSITION_MODEL_VERSION",
		"OPPONENT_MODEL_URL", "OPPONENT_MODEL_NAME", "OPPONENT_MODEL_VERSION",
	}
	clear := func(t *testing.T) {
		t.Helper()
		for _, variable := range variables {
			t.Setenv(variable, "")
		}
	}

	t.Run("disabled", func(t *testing.T) {
		clear(t)
		config, err := loadPositionModelConfig()
		if err != nil || config != nil {
			t.Fatalf("expected no model configuration, got %+v: %v", config, err)
		}
	})

	t.Run("preferred", func(t *testing.T) {
		clear(t)
		t.Setenv("POSITION_MODEL_URL", "https://model.example/v1/positions")
		t.Setenv("POSITION_MODEL_NAME", "trajectory-policy")
		t.Setenv("POSITION_MODEL_VERSION", "2")
		config, err := loadPositionModelConfig()
		if err != nil || config == nil || config.deprecatedAlias || config.name != "trajectory-policy" {
			t.Fatalf("unexpected preferred configuration %+v: %v", config, err)
		}
	})

	t.Run("deprecated alias", func(t *testing.T) {
		clear(t)
		t.Setenv("OPPONENT_MODEL_URL", "https://model.example/v1/positions")
		t.Setenv("OPPONENT_MODEL_NAME", "old-policy")
		t.Setenv("OPPONENT_MODEL_VERSION", "1")
		config, err := loadPositionModelConfig()
		if err != nil || config == nil || !config.deprecatedAlias || config.name != "old-policy" {
			t.Fatalf("unexpected deprecated-alias configuration %+v: %v", config, err)
		}
	})

	t.Run("partial deprecated alias", func(t *testing.T) {
		clear(t)
		t.Setenv("OPPONENT_MODEL_URL", "https://model.example/v1/positions")
		if _, err := loadPositionModelConfig(); err == nil {
			t.Fatal("expected partial deprecated alias configuration to fail")
		}
	})

	t.Run("partial preferred", func(t *testing.T) {
		clear(t)
		t.Setenv("POSITION_MODEL_URL", "https://model.example/v1/positions")
		if _, err := loadPositionModelConfig(); err == nil {
			t.Fatal("expected partial preferred configuration to fail")
		}
	})

	t.Run("mixed names", func(t *testing.T) {
		clear(t)
		t.Setenv("POSITION_MODEL_URL", "https://model.example/v1/positions")
		t.Setenv("POSITION_MODEL_NAME", "trajectory-policy")
		t.Setenv("POSITION_MODEL_VERSION", "2")
		t.Setenv("OPPONENT_MODEL_NAME", "old-policy")
		if _, err := loadPositionModelConfig(); err == nil {
			t.Fatal("expected mixed configuration names to fail")
		}
	})
}
