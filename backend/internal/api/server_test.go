package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func testServer() *Server {
	moment := model.Moment{
		ID: "m1", Slug: "test", Title: "Test moment", Seed: 1, MaxTurns: 2,
		ControlledUnitID: "blue", ReasonTags: []string{"clutch"},
		Units: []model.Unit{
			{ID: "blue", Team: "blue", Position: model.Point{X: 30, Y: 50}, HP: 100, MaxHP: 100, Alive: true},
			{ID: "red", Team: "red", Position: model.Point{X: 45, Y: 50}, HP: 100, MaxHP: 100, Alive: true},
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
	turn := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/turns", `{"action":{"type":"contest"}}`)
	if turn.Code != http.StatusOK {
		t.Fatalf("turn: %d %s", turn.Code, turn.Body.String())
	}
	reset := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/reset", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d", reset.Code)
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
