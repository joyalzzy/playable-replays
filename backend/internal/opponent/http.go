package opponent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
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
	Positions *[]wirePosition `json:"positions"`
}

type wirePosition struct {
	UnitID   *string    `json:"unitId"`
	Position *wirePoint `json:"position"`
}

type wirePoint struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

func NewHTTPModel(rawURL, modelName, modelVersion string, client *http.Client) (*HTTPModel, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("opponent model URL must be an absolute HTTP(S) URL")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, errors.New("opponent model URL must use HTTP(S)")
	}
	modelName = strings.TrimSpace(modelName)
	modelVersion = strings.TrimSpace(modelVersion)
	if modelName == "" || len(modelName) > maxIdentityBytes {
		return nil, fmt.Errorf("opponent model name must contain 1 to %d bytes", maxIdentityBytes)
	}
	if modelVersion == "" || len(modelVersion) > maxIdentityBytes {
		return nil, fmt.Errorf("opponent model version must contain 1 to %d bytes", maxIdentityBytes)
	}
	if client == nil {
		client = &http.Client{Timeout: 1500 * time.Millisecond}
	}
	return &HTTPModel{endpoint: endpoint, client: client, name: modelName, version: modelVersion}, nil
}

func (m *HTTPModel) NextPositions(ctx context.Context, snapshot engine.ModelSnapshot) (engine.ModelResult, error) {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return engine.ModelResult{}, fmt.Errorf("encode opponent snapshot: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return engine.ModelResult{}, fmt.Errorf("create opponent request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := m.client.Do(req)
	if err != nil {
		return engine.ModelResult{}, errors.New("opponent model request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxResponseBytes))
		return engine.ModelResult{}, fmt.Errorf("opponent model returned status %d", res.StatusCode)
	}

	limited := io.LimitReader(res.Body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return engine.ModelResult{}, errors.New("read opponent model response")
	}
	if len(data) > maxResponseBytes {
		return engine.ModelResult{}, errors.New("opponent model response is too large")
	}
	var decoded response
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return engine.ModelResult{}, errors.New("opponent model returned malformed JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return engine.ModelResult{}, errors.New("opponent model response must contain one JSON value")
	}
	if decoded.Positions == nil {
		return engine.ModelResult{}, errors.New("opponent model response must contain a positions array")
	}
	if len(*decoded.Positions) > engine.MaxSnapshotUnits {
		return engine.ModelResult{}, errors.New("opponent model returned too many positions")
	}
	positions := make([]engine.PositionSuggestion, 0, len(*decoded.Positions))
	seen := make(map[string]struct{}, len(*decoded.Positions))
	for _, wire := range *decoded.Positions {
		if wire.UnitID == nil || *wire.UnitID == "" {
			return engine.ModelResult{}, errors.New("opponent model returned an empty unit ID")
		}
		if wire.Position == nil || wire.Position.X == nil || wire.Position.Y == nil {
			return engine.ModelResult{}, errors.New("opponent model returned an incomplete position")
		}
		suggestion := engine.PositionSuggestion{
			UnitID:   *wire.UnitID,
			Position: model.Point{X: *wire.Position.X, Y: *wire.Position.Y},
		}
		if _, duplicate := seen[suggestion.UnitID]; duplicate {
			return engine.ModelResult{}, errors.New("opponent model returned a duplicate unit ID")
		}
		seen[suggestion.UnitID] = struct{}{}
		if !finite(suggestion.Position.X) || !finite(suggestion.Position.Y) {
			return engine.ModelResult{}, errors.New("opponent model returned a non-finite position")
		}
		if suggestion.Position.X < snapshot.MapBounds.MinX || suggestion.Position.X > snapshot.MapBounds.MaxX ||
			suggestion.Position.Y < snapshot.MapBounds.MinY || suggestion.Position.Y > snapshot.MapBounds.MaxY {
			return engine.ModelResult{}, errors.New("opponent model returned an out-of-bounds position")
		}
		positions = append(positions, suggestion)
	}
	return engine.ModelResult{
		ModelName: m.name, ModelVersion: m.version, Positions: positions,
	}, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
