package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

type apiOpponentModel struct {
	called bool
}

func (stub *apiOpponentModel) NextPositions(_ context.Context, _ engine.OpponentSnapshot) ([]engine.PositionSuggestion, error) {
	stub.called = true
	return []engine.PositionSuggestion{{UnitID: "red", Position: model.Point{X: 100, Y: 50}}}, nil
}

func testServer() *Server {
	moment := model.Moment{
		ID: "m1", Slug: "test", Title: "Test moment", Seed: 1, MaxTurns: 2,
		ControlledUnitID: "blue", ReasonTags: []string{"clutch"},
		Units: []model.Unit{
			{ID: "blue", Team: "blue", Role: "carry", Class: model.ClassMarksman, Position: model.Point{X: 30, Y: 50}, HP: 80, MaxHP: 90, Alive: true},
			{ID: "red", Team: "red", Role: "tank", Class: model.ClassTank, Position: model.Point{X: 45, Y: 50}, HP: 120, MaxHP: 160, Alive: true},
		},
	}
	return New([]model.Moment{moment}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	return result
}

func TestJourney(t *testing.T) {
	handler := testServer().Handler()
	list := request(t, handler, http.MethodGet, "/api/v1/moments", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list: %d", list.Code)
	}
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.ControlledUnitID != "blue" || session.Units[0].MoveRange != 11 || session.Units[1].MaxHP != 160 {
		t.Fatalf("class/session contract was not serialized: %+v", session)
	}
	if len(session.LegalActions) != 6 {
		t.Fatalf("expected expanded legal action set, got %v", session.LegalActions)
	}
	turn := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/turns", `{"action":{"type":"contest"}}`)
	if turn.Code != http.StatusOK {
		t.Fatalf("turn: %d %s", turn.Code, turn.Body.String())
	}
	reset := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/reset", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d", reset.Code)
	}
}

func TestRejectsOverRangeMovementWithoutAdvancingTurn(t *testing.T) {
	handler := testServer().Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	result := request(
		t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/turns",
		`{"action":{"type":"move","target":{"x":100,"y":50}}}`,
	)
	if result.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", result.Code, result.Body.String())
	}
	current := request(t, handler, http.MethodGet, "/api/v1/sessions/"+session.ID, "")
	if err := json.Unmarshal(current.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Turn != 0 || session.Units[0].Position != (model.Point{X: 30, Y: 50}) {
		t.Fatalf("illegal movement mutated session: %+v", session)
	}
}

func TestServerUsesOpponentPositionModel(t *testing.T) {
	stub := &apiOpponentModel{}
	base := testServer()
	server := NewWithOpponentModel(base.ordered, slog.New(slog.NewTextHandler(io.Discard, nil)), stub)
	handler := server.Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	turn := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/turns", `{"action":{"type":"hold"}}`)
	if turn.Code != http.StatusOK {
		t.Fatalf("turn: %d %s", turn.Code, turn.Body.String())
	}
	if err := json.Unmarshal(turn.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if !stub.called || session.Units[1].Position != (model.Point{X: 52, Y: 50}) {
		t.Fatalf("opponent connector was not applied with tank limit: %+v", session)
	}
}

func TestStructuredErrors(t *testing.T) {
	result := request(t, testServer().Handler(), http.MethodGet, "/api/v1/sessions/nope", "")
	if result.Code != http.StatusNotFound || result.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected error response: %d %q", result.Code, result.Header().Get("Content-Type"))
	}
	var response model.ErrorResponse
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil || response.Error.Code == "" {
		t.Fatalf("expected structured JSON error: %v", err)
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	result := request(t, testServer().Handler(), http.MethodPost, "/api/v1/sessions", `{"momentId":"m1","admin":true}`)
	if result.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", result.Code)
	}
}

func TestStableMomentIDIncludesWindow(t *testing.T) {
	if StableMomentID("Team Fight", 42) == StableMomentID("Team Fight", 43) {
		t.Fatal("moment IDs from distinct windows collided")
	}
}
