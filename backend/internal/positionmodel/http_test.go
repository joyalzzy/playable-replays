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

func testBotSnapshot() engine.BotSnapshot {
	return engine.BotSnapshot{
		SchemaVersion: engine.BotModelSchemaVersion, StateScope: "authoritative_server_state",
		SessionID: "session-1", MomentID: "moment-1", Turn: 1,
		MapBounds:        engine.MapBounds{MinX: 0, MaxX: 100, MinY: 0, MaxY: 100},
		ControlledUnitID: "blue",
		LegalActions:     []string{"move", "hold", "contest", "retreat"},
		Projectiles: []model.Projectile{{
			ID: "projectile-1", Team: "red", SourceUnitID: "red", TargetUnitID: "blue",
			Position: model.Point{X: 50, Y: 50}, Target: model.Point{X: 30, Y: 50}, Damage: 45,
		}},
		Units: []engine.BotSnapshotUnit{
			{ID: "blue", Team: "blue", Role: "carry", Class: model.ClassMarksman, Policy: "controlled", Position: model.Point{X: 30, Y: 50}, HP: 70, MaxHP: 90, MoveRange: 11, AttackRange: 28, Alive: true, Visible: true},
			{ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Policy: "support", Position: model.Point{X: 35, Y: 50}, HP: 100, MaxHP: 110, MoveRange: 8, AttackRange: 20, Alive: true, Visible: true},
			{ID: "red", Team: "red", Role: "frontline", Class: model.ClassTank, Policy: "aggressive", Position: model.Point{X: 50, Y: 50}, HP: 120, MaxHP: 160, MoveRange: 7, AttackRange: 10, Alive: true, Visible: true},
		},
	}
}

func TestHTTPModelSendsSnapshotAndDecodesActions(t *testing.T) {
	received := make(chan engine.BotSnapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/actions" ||
			request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var snapshot engine.BotSnapshot
		if err := json.NewDecoder(request.Body).Decode(&snapshot); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- snapshot
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"actions":[{"unitId":"blue-support","action":{"type":"hold"}},{"unitId":"red","action":{"type":"move","target":{"x":75,"y":40}}}]}`))
	}))
	defer server.Close()

	connector, err := NewHTTPModel(server.URL+"/v1/actions", "test-policy", "2", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := connector.NextActions(context.Background(), testBotSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelName != "test-policy" || result.ModelVersion != "2" || len(result.Actions) != 2 {
		t.Fatalf("unexpected bot-model response: %+v", result)
	}
	if result.Actions[0].UnitID != "blue-support" || result.Actions[0].Action.Type != "hold" || result.Actions[0].Action.Target != nil {
		t.Fatalf("hold action changed in transit: %+v", result.Actions[0])
	}
	if result.Actions[1].UnitID != "red" || result.Actions[1].Action.Type != "move" ||
		result.Actions[1].Action.Target == nil || *result.Actions[1].Action.Target != (model.Point{X: 75, Y: 40}) {
		t.Fatalf("move action changed in transit: %+v", result.Actions[1])
	}
	select {
	case snapshot := <-received:
		if snapshot.SchemaVersion != engine.BotModelSchemaVersion || snapshot.SessionID != "session-1" ||
			len(snapshot.Units) != 3 || len(snapshot.Projectiles) != 1 || len(snapshot.LegalActions) != 4 {
			t.Fatalf("authoritative snapshot changed in transit: %+v", snapshot)
		}
	default:
		t.Fatal("model endpoint did not receive a snapshot")
	}
}

func TestHTTPModelRejectsOversizedRequestBeforeSending(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"actions":[]}`))
	}))
	defer server.Close()
	connector, err := NewHTTPModel(server.URL, "test-policy", "2", nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testBotSnapshot()
	snapshot.SessionID = strings.Repeat("x", maxRequestBytes)
	if _, err := connector.NextActions(context.Background(), snapshot); err == nil {
		t.Fatal("expected oversized privileged snapshot to be rejected")
	}
	if requests != 0 {
		t.Fatalf("oversized request reached the model endpoint %d times", requests)
	}
}

func TestHTTPModelRejectsInvalidResponses(t *testing.T) {
	tooMany := make([]engine.BotActionSuggestion, engine.MaxSnapshotUnits+1)
	for index := range tooMany {
		tooMany[index] = engine.BotActionSuggestion{
			UnitID: "red-" + strings.Repeat("x", index+1), Action: model.Action{Type: "hold"},
		}
	}
	tooManyBody, err := json.Marshal(map[string]any{"actions": tooMany})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		status int
		body   string
	}{
		"created":                {status: http.StatusCreated, body: `{"actions":[]}`},
		"non success":            {status: http.StatusBadGateway, body: `{"error":"down"}`},
		"malformed":              {status: http.StatusOK, body: `{"actions":[`},
		"top level null":         {status: http.StatusOK, body: `null`},
		"null actions":           {status: http.StatusOK, body: `{"actions":null}`},
		"unknown envelope field": {status: http.StatusOK, body: `{"actions":[],"teleport":true}`},
		"missing unit id":        {status: http.StatusOK, body: `{"actions":[{"action":{"type":"hold"}}]}`},
		"empty unit id":          {status: http.StatusOK, body: `{"actions":[{"unitId":" ","action":{"type":"hold"}}]}`},
		"missing action":         {status: http.StatusOK, body: `{"actions":[{"unitId":"red"}]}`},
		"missing action type":    {status: http.StatusOK, body: `{"actions":[{"unitId":"red","action":{}}]}`},
		"unknown action field":   {status: http.StatusOK, body: `{"actions":[{"unitId":"red","action":{"type":"hold","confidence":1}}]}`},
		"incomplete target":      {status: http.StatusOK, body: `{"actions":[{"unitId":"red","action":{"type":"move","target":{"x":1}}}]}`},
		"duplicate unit":         {status: http.StatusOK, body: `{"actions":[{"unitId":"red","action":{"type":"hold"}},{"unitId":"red","action":{"type":"contest"}}]}`},
		"multiple values":        {status: http.StatusOK, body: `{"actions":[]} {}`},
		"too many":               {status: http.StatusOK, body: string(tooManyBody)},
		"oversized":              {status: http.StatusOK, body: `{"actions":[]}` + strings.Repeat(" ", maxResponseBytes)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			connector, err := NewHTTPModel(server.URL, "test-policy", "2", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := connector.NextActions(context.Background(), testBotSnapshot()); err == nil {
				t.Fatal("expected response to be rejected")
			}
		})
	}
}

func TestHTTPModelLeavesSemanticActionValidationToEngine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"actions":[{"unitId":"red","action":{"type":"outplay","target":{"x":101,"y":2}}}]}`))
	}))
	defer server.Close()
	connector, err := NewHTTPModel(server.URL, "test-policy", "2", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := connector.NextActions(context.Background(), testBotSnapshot())
	if err != nil || len(result.Actions) != 1 || result.Actions[0].Action.Type != "outplay" ||
		result.Actions[0].Action.Target == nil || result.Actions[0].Action.Target.X != 101 {
		t.Fatalf("connector should preserve syntactically complete actions for authoritative engine validation: %+v: %v", result, err)
	}
}

func TestHTTPModelAllowsExplicitEmptyActions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"actions":[]}`))
	}))
	defer server.Close()
	connector, err := NewHTTPModel(server.URL, "test-policy", "2", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := connector.NextActions(context.Background(), testBotSnapshot())
	if err != nil || len(result.Actions) != 0 {
		t.Fatalf("expected an explicit empty response, got %+v: %v", result, err)
	}
}

func TestNewHTTPModelValidatesURL(t *testing.T) {
	for _, endpoint := range []string{"", "relative/path", "file:///tmp/model", "://bad"} {
		if _, err := NewHTTPModel(endpoint, "test-policy", "2", nil); err == nil {
			t.Fatalf("expected %q to be rejected", endpoint)
		}
	}
}

func TestNewHTTPModelRequiresBoundedIdentity(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "", version: "2"},
		{name: "test-policy", version: ""},
		{name: strings.Repeat("x", maxIdentityBytes+1), version: "2"},
		{name: "test-policy", version: strings.Repeat("x", maxIdentityBytes+1)},
	}
	for _, test := range tests {
		if _, err := NewHTTPModel("https://model.example/v1/actions", test.name, test.version, nil); err == nil {
			t.Fatalf("expected identity name=%q version=%q to be rejected", test.name, test.version)
		}
	}
}

func TestNewHTTPModelUsesNineSecondDefaultTimeout(t *testing.T) {
	connector, err := NewHTTPModel("https://model.example/v1/actions", "test-policy", "2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if connector.client.Timeout != 9*time.Second {
		t.Fatalf("default client timeout = %s, want 9s", connector.client.Timeout)
	}
}

func TestHTTPModelHonorsCustomClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte(`{"actions":[]}`))
	}))
	defer server.Close()
	connector, err := NewHTTPModel(server.URL, "test-policy", "2", &http.Client{Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.NextActions(context.Background(), testBotSnapshot()); err == nil {
		t.Fatal("expected custom timeout to activate deterministic fallback")
	}
}
