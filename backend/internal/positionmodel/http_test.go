package positionmodel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func testSnapshot() engine.ModelSnapshot {
	return engine.ModelSnapshot{
		SchemaVersion: engine.PositionModelSchemaVersion, StateScope: "authoritative_server_state",
		SessionID: "session-1", MomentID: "moment-1", Turn: 1,
		MapBounds:        engine.MapBounds{MinX: 0, MaxX: 100, MinY: 0, MaxY: 100},
		ControlledUnitID: "blue",
		Units: []engine.ModelSnapshotUnit{
			{ID: "blue", Team: "blue", Role: "carry", Class: model.ClassMarksman, Position: model.Point{X: 30, Y: 50}, HP: 70, MaxHP: 90, MoveRange: 11, AttackRange: 28, Alive: true, Visible: true},
			{ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Position: model.Point{X: 35, Y: 50}, HP: 100, MaxHP: 110, MoveRange: 8, AttackRange: 20, Alive: true, Visible: true},
			{ID: "red", Team: "red", Role: "frontline", Class: model.ClassTank, Position: model.Point{X: 50, Y: 50}, HP: 120, MaxHP: 160, MoveRange: 7, AttackRange: 10, Alive: true, Visible: true},
		},
	}
}

func TestHTTPModelSendsSnapshotAndDecodesPositions(t *testing.T) {
	received := make(chan engine.ModelSnapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var snapshot engine.ModelSnapshot
		if err := json.NewDecoder(request.Body).Decode(&snapshot); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- snapshot
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"positions":[{"unitId":"blue-support","position":{"x":42,"y":51}},{"unitId":"red","position":{"x":75,"y":40}}]}`))
	}))
	defer server.Close()

	connector, err := NewHTTPModel(server.URL, "trajectory-policy", "2026.08.04", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := connector.NextPositions(context.Background(), testSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelName != "trajectory-policy" || result.ModelVersion != "2026.08.04" ||
		len(result.Positions) != 2 || result.Positions[0].UnitID != "blue-support" ||
		result.Positions[0].Position != (model.Point{X: 42, Y: 51}) ||
		result.Positions[1].UnitID != "red" || result.Positions[1].Position != (model.Point{X: 75, Y: 40}) {
		t.Fatalf("unexpected model result: %+v", result)
	}
	snapshot := <-received
	if snapshot.SchemaVersion != "1.1" || snapshot.StateScope != "authoritative_server_state" ||
		snapshot.Turn != 1 || len(snapshot.Units) != 3 || snapshot.Units[1].Role != "support" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestHTTPModelRejectsOversizedRequestBeforeSending(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"positions":[]}`))
	}))
	defer server.Close()

	connector, err := NewHTTPModel(server.URL, "test-policy", "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot()
	snapshot.SessionID = strings.Repeat("x", maxRequestBytes)
	if _, err := connector.NextPositions(context.Background(), snapshot); err == nil {
		t.Fatal("expected oversized privileged snapshot to be rejected")
	}
	if requests != 0 {
		t.Fatalf("oversized request reached the model endpoint %d times", requests)
	}
}

func TestHTTPModelRejectsInvalidResponses(t *testing.T) {
	tooMany := make([]engine.PositionSuggestion, engine.MaxSnapshotUnits+1)
	for index := range tooMany {
		tooMany[index] = engine.PositionSuggestion{UnitID: "red-" + strings.Repeat("x", index+1), Position: model.Point{X: 1, Y: 1}}
	}
	tooManyBody, err := json.Marshal(map[string]any{"positions": tooMany})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		status int
		body   string
	}{
		"created":            {status: http.StatusCreated, body: `{"positions":[]}`},
		"non success":        {status: http.StatusBadGateway, body: `{"error":"down"}`},
		"malformed":          {status: http.StatusOK, body: `{"positions":[`},
		"top level null":     {status: http.StatusOK, body: `null`},
		"null positions":     {status: http.StatusOK, body: `{"positions":null}`},
		"unknown field":      {status: http.StatusOK, body: `{"positions":[],"teleport":true}`},
		"missing unit id":    {status: http.StatusOK, body: `{"positions":[{"position":{"x":1,"y":2}}]}`},
		"missing position":   {status: http.StatusOK, body: `{"positions":[{"unitId":"red"}]}`},
		"missing coordinate": {status: http.StatusOK, body: `{"positions":[{"unitId":"red","position":{"x":1}}]}`},
		"duplicate unit":     {status: http.StatusOK, body: `{"positions":[{"unitId":"red","position":{"x":1,"y":2}},{"unitId":"red","position":{"x":2,"y":3}}]}`},
		"outside map":        {status: http.StatusOK, body: `{"positions":[{"unitId":"red","position":{"x":101,"y":2}}]}`},
		"non finite":         {status: http.StatusOK, body: `{"positions":[{"unitId":"red","position":{"x":NaN,"y":2}}]}`},
		"multiple values":    {status: http.StatusOK, body: `{"positions":[]} {}`},
		"too many":           {status: http.StatusOK, body: string(tooManyBody)},
		"oversized":          {status: http.StatusOK, body: `{"positions":[]}` + strings.Repeat(" ", maxResponseBytes)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			connector, err := NewHTTPModel(server.URL, "test-policy", "1", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := connector.NextPositions(context.Background(), testSnapshot()); err == nil {
				t.Fatal("expected response to be rejected")
			}
		})
	}
}

func TestHTTPModelAllowsExplicitEmptyPositions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"positions":[]}`))
	}))
	defer server.Close()
	connector, err := NewHTTPModel(server.URL, "test-policy", "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := connector.NextPositions(context.Background(), testSnapshot())
	if err != nil || len(result.Positions) != 0 {
		t.Fatalf("expected an explicit empty response, got %+v: %v", result, err)
	}
}

func TestNewHTTPModelValidatesURL(t *testing.T) {
	for _, endpoint := range []string{"", "relative/path", "file:///tmp/model", "://bad"} {
		if _, err := NewHTTPModel(endpoint, "test-policy", "1", nil); err == nil {
			t.Fatalf("expected %q to be rejected", endpoint)
		}
	}
}

func TestNewHTTPModelRequiresBoundedIdentity(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "", version: "1"},
		{name: "test-policy", version: ""},
		{name: strings.Repeat("x", maxIdentityBytes+1), version: "1"},
		{name: "test-policy", version: strings.Repeat("x", maxIdentityBytes+1)},
	}
	for _, test := range tests {
		if _, err := NewHTTPModel("https://model.example/v1/positions", test.name, test.version, nil); err == nil {
			t.Fatalf("expected identity name=%q version=%q to be rejected", test.name, test.version)
		}
	}
}

func TestHTTPModelHonorsClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte(`{"positions":[]}`))
	}))
	defer server.Close()
	connector, err := NewHTTPModel(server.URL, "test-policy", "1", &http.Client{Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.NextPositions(context.Background(), testSnapshot()); err == nil {
		t.Fatal("expected timeout to activate fallback")
	}
}
