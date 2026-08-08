package main

import "testing"

func TestLoadBotModelConfig(t *testing.T) {
	variables := []string{"BOT_MODEL_URL", "BOT_MODEL_NAME", "BOT_MODEL_VERSION"}
	clear := func(t *testing.T) {
		t.Helper()
		for _, variable := range variables {
			t.Setenv(variable, "")
		}
	}

	t.Run("disabled", func(t *testing.T) {
		clear(t)
		config, err := loadBotModelConfig()
		if err != nil || config != nil {
			t.Fatalf("expected no model configuration, got %+v: %v", config, err)
		}
	})

	t.Run("complete", func(t *testing.T) {
		clear(t)
		t.Setenv("BOT_MODEL_URL", "https://model.example/v1/actions")
		t.Setenv("BOT_MODEL_NAME", "action-policy")
		t.Setenv("BOT_MODEL_VERSION", "2")
		config, err := loadBotModelConfig()
		if err != nil || config == nil {
			t.Fatalf("expected complete bot-model configuration, got %+v: %v", config, err)
		}
		if config.endpoint != "https://model.example/v1/actions" || config.name != "action-policy" || config.version != "2" {
			t.Fatalf("unexpected bot-model configuration: %+v", config)
		}
	})

	for _, missing := range variables {
		missing := missing
		t.Run("missing "+missing, func(t *testing.T) {
			clear(t)
			t.Setenv("BOT_MODEL_URL", "https://model.example/v1/actions")
			t.Setenv("BOT_MODEL_NAME", "action-policy")
			t.Setenv("BOT_MODEL_VERSION", "2")
			t.Setenv(missing, "")
			if _, err := loadBotModelConfig(); err == nil {
				t.Fatalf("expected incomplete configuration without %s to fail", missing)
			}
		})
	}
}
