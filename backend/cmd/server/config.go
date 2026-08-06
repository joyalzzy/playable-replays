package main

import (
	"errors"
	"os"
)

type positionModelConfig struct {
	endpoint        string
	name            string
	version         string
	deprecatedAlias bool
}

func loadPositionModelConfig() (*positionModelConfig, error) {
	preferred := positionModelConfig{
		endpoint: os.Getenv("POSITION_MODEL_URL"),
		name:     os.Getenv("POSITION_MODEL_NAME"),
		version:  os.Getenv("POSITION_MODEL_VERSION"),
	}
	deprecated := positionModelConfig{
		endpoint:        os.Getenv("OPPONENT_MODEL_URL"),
		name:            os.Getenv("OPPONENT_MODEL_NAME"),
		version:         os.Getenv("OPPONENT_MODEL_VERSION"),
		deprecatedAlias: true,
	}

	preferredSet := preferred.anySet()
	deprecatedSet := deprecated.anySet()
	if preferredSet && deprecatedSet {
		return nil, errors.New("position model configuration must not mix POSITION_MODEL_* and deprecated OPPONENT_MODEL_* variables")
	}
	if preferredSet {
		if !preferred.complete() {
			return nil, errors.New("POSITION_MODEL_URL, POSITION_MODEL_NAME, and POSITION_MODEL_VERSION must be set together")
		}
		return &preferred, nil
	}
	if deprecatedSet {
		if !deprecated.complete() {
			return nil, errors.New("OPPONENT_MODEL_URL, OPPONENT_MODEL_NAME, and OPPONENT_MODEL_VERSION must be set together")
		}
		return &deprecated, nil
	}
	return nil, nil
}

func (config positionModelConfig) anySet() bool {
	return config.endpoint != "" || config.name != "" || config.version != ""
}

func (config positionModelConfig) complete() bool {
	return config.endpoint != "" && config.name != "" && config.version != ""
}
