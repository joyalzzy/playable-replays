// Package positionmodel retains its historical import path while implementing
// the version 2.0 bot-action contract. New code should describe this dependency
// as the bot model, not as a position-only model.
package positionmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	maxRequestBytes  = 64 << 10
	maxResponseBytes = 64 << 10
	maxIdentityBytes = 128
)

type HTTPModel struct {
	endpoint *url.URL
	client   *http.Client
	name     string
	version  string
}

type response struct {
	Actions *[]wireAction `json:"actions"`
}

type wireAction struct {
	UnitID *string     `json:"unitId"`
	Action *wireIntent `json:"action"`
}

type wireIntent struct {
	Type   *string    `json:"type"`
	Target *wirePoint `json:"target,omitempty"`
}

type wirePoint struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

func NewHTTPModel(rawURL, modelName, modelVersion string, client *http.Client) (*HTTPModel, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("bot model URL must be an absolute HTTP(S) URL")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, errors.New("bot model URL must use HTTP(S)")
	}
	modelName = strings.TrimSpace(modelName)
	modelVersion = strings.TrimSpace(modelVersion)
	if modelName == "" || len(modelName) > maxIdentityBytes {
		return nil, fmt.Errorf("bot model name must contain 1 to %d bytes", maxIdentityBytes)
	}
	if modelVersion == "" || len(modelVersion) > maxIdentityBytes {
		return nil, fmt.Errorf("bot model version must contain 1 to %d bytes", maxIdentityBytes)
	}
	if client == nil {
		client = &http.Client{Timeout: 9 * time.Second}
	}
	return &HTTPModel{endpoint: endpoint, client: client, name: modelName, version: modelVersion}, nil
}

func (m *HTTPModel) NextActions(ctx context.Context, snapshot engine.BotSnapshot) (engine.BotModelResult, error) {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return engine.BotModelResult{}, fmt.Errorf("encode bot-model snapshot: %w", err)
	}
	if len(body) > maxRequestBytes {
		return engine.BotModelResult{}, errors.New("bot model request is too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return engine.BotModelResult{}, fmt.Errorf("create bot-model request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := m.client.Do(req)
	if err != nil {
		return engine.BotModelResult{}, errors.New("bot model request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxResponseBytes))
		return engine.BotModelResult{}, fmt.Errorf("bot model returned status %d", res.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return engine.BotModelResult{}, errors.New("read bot model response")
	}
	if len(data) > maxResponseBytes {
		return engine.BotModelResult{}, errors.New("bot model response is too large")
	}
	var decoded response
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return engine.BotModelResult{}, errors.New("bot model returned malformed JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return engine.BotModelResult{}, errors.New("bot model response must contain one JSON value")
	}
	if decoded.Actions == nil {
		return engine.BotModelResult{}, errors.New("bot model response must contain an actions array")
	}
	if len(*decoded.Actions) > engine.MaxSnapshotUnits {
		return engine.BotModelResult{}, errors.New("bot model returned too many actions")
	}

	actions := make([]engine.BotActionSuggestion, 0, len(*decoded.Actions))
	seen := make(map[string]struct{}, len(*decoded.Actions))
	for _, wire := range *decoded.Actions {
		if wire.UnitID == nil || strings.TrimSpace(*wire.UnitID) == "" || wire.Action == nil || wire.Action.Type == nil {
			return engine.BotModelResult{}, errors.New("bot model returned an incomplete action")
		}
		if _, duplicate := seen[*wire.UnitID]; duplicate {
			return engine.BotModelResult{}, errors.New("bot model returned a duplicate unit ID")
		}
		seen[*wire.UnitID] = struct{}{}
		action := model.Action{Type: *wire.Action.Type}
		if wire.Action.Target != nil {
			if wire.Action.Target.X == nil || wire.Action.Target.Y == nil {
				return engine.BotModelResult{}, errors.New("bot model returned an incomplete target")
			}
			target := model.Point{X: *wire.Action.Target.X, Y: *wire.Action.Target.Y}
			action.Target = &target
		}
		actions = append(actions, engine.BotActionSuggestion{UnitID: *wire.UnitID, Action: action})
	}
	return engine.BotModelResult{ModelName: m.name, ModelVersion: m.version, Actions: actions}, nil
}
