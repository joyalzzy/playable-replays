package main

import (
	"errors"
	"os"
)

type botModelConfig struct {
	endpoint string
	name     string
	version  string
}

func loadBotModelConfig() (*botModelConfig, error) {
	config := botModelConfig{
		endpoint: os.Getenv("BOT_MODEL_URL"),
		name:     os.Getenv("BOT_MODEL_NAME"),
		version:  os.Getenv("BOT_MODEL_VERSION"),
	}
	if config.anySet() {
		if !config.complete() {
			return nil, errors.New("BOT_MODEL_URL, BOT_MODEL_NAME, and BOT_MODEL_VERSION must be set together")
		}
		return &config, nil
	}
	return nil, nil
}

func (config botModelConfig) anySet() bool {
	return config.endpoint != "" || config.name != "" || config.version != ""
}

func (config botModelConfig) complete() bool {
	return config.endpoint != "" && config.name != "" && config.version != ""
}
