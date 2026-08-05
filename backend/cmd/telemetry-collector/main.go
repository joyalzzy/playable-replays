// Command telemetry-collector replays a normalized, authorized telemetry file
// into the local API. It is intentionally source-agnostic: vendor-specific
// capture adapters remain outside the simulator trust boundary.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
	"github.com/joyalzzy/playable-replays/backend/internal/telemetry"
)

type options struct {
	input  string
	api    string
	source string
	rate   float64
}

func main() {
	var config options
	flag.StringVar(&config.input, "input", "", "path to normalized telemetry JSON")
	flag.StringVar(&config.api, "api", "http://127.0.0.1:8080", "local Playable Replays API")
	flag.StringVar(&config.source, "source", "synthetic", "synthetic or authorized")
	flag.Float64Var(&config.rate, "rate", 4, "replay speed multiplier; 0 sends immediately")
	flag.Parse()
	if err := collect(context.Background(), http.DefaultClient, config, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "telemetry collector:", err)
		os.Exit(1)
	}
}

func collect(ctx context.Context, client *http.Client, config options, output io.Writer) error {
	if strings.TrimSpace(config.input) == "" {
		return errors.New("--input is required")
	}
	if config.rate < 0 {
		return errors.New("--rate cannot be negative")
	}
	input, err := os.Open(config.input)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer input.Close()
	document, err := telemetry.ReadDocument(input)
	if err != nil {
		return err
	}

	baseURL := strings.TrimRight(config.api, "/")
	var started model.CreateTelemetryMatchResponse
	if err := exchangeJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/telemetry/matches", "", model.CreateTelemetryMatchRequest{Source: config.source, Consent: true}, &started); err != nil {
		return fmt.Errorf("start match: %w", err)
	}
	fmt.Fprintf(output, "Started %s from %d normalized frames.\n", started.Match.ID, len(document.Frames))

	previousSecond := document.Frames[0].Second
	for sequence, frame := range document.Frames {
		if sequence > 0 && config.rate > 0 {
			delta := frame.Second - previousSecond
			delay := time.Duration(float64(time.Second) * float64(delta) / config.rate)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		batch := model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: sequence, Frames: []model.LiveTelemetryFrame{frame}}
		var current model.TelemetryMatch
		if err := exchangeJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/telemetry/matches/"+started.Match.ID+"/frames", started.CollectorToken, batch, &current); err != nil {
			return fmt.Errorf("send frame %d: %w", sequence, err)
		}
		previousSecond = frame.Second
	}
	var finalized model.TelemetryMatch
	if err := exchangeJSON(ctx, client, http.MethodPost, baseURL+"/api/v1/telemetry/matches/"+started.Match.ID+"/finish", started.CollectorToken, nil, &finalized); err != nil {
		return fmt.Errorf("finish match: %w", err)
	}
	fmt.Fprintf(output, "Finalized %s with %d detected highlight candidate(s).\n", finalized.ID, len(finalized.Candidates))
	for _, candidate := range finalized.Candidates {
		fmt.Fprintf(output, "- %s: %s, score %.4f, analyst draft %s\n", candidate.ID, candidate.Category, candidate.Detection.Score, candidate.DraftStatus)
	}
	return nil
}

func exchangeJSON(ctx context.Context, client *http.Client, method, url, token string, body, destination any) error {
	var encoded io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		encoded = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, encoded)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if destination == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(destination); err != nil {
		return fmt.Errorf("decode API response: %w", err)
	}
	return nil
}
