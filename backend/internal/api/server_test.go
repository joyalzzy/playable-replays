package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func testServer() *Server {
	moment := model.Moment{
		ID: "m1", Slug: "test", Title: "Test moment", Seed: 1, MaxTurns: 2,
		ControlledUnitID: "blue", ReasonTags: []string{"clutch"},
		Authoring: model.ScenarioAuthoring{Category: "positioning", SkillLevel: "beginner"},
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
	var listed struct {
		Moments []model.MomentSummary `json:"moments"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Moments) != 1 || listed.Moments[0].Category != "positioning" || listed.Moments[0].SkillLevel != "beginner" {
		t.Fatalf("list response omitted authoring dimensions: %+v", listed.Moments)
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

func TestAuthoredLibraryJourney(t *testing.T) {
	moments, err := fixtures.Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(moments, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	listed := request(t, handler, http.MethodGet, "/api/v1/moments", "")
	var payload struct {
		Moments []model.MomentSummary `json:"moments"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if listed.Code != http.StatusOK || len(payload.Moments) != 12 {
		t.Fatalf("expected twelve authored summaries, got %d with status %d", len(payload.Moments), listed.Code)
	}
	for _, summary := range payload.Moments {
		if summary.Category == "" || summary.SkillLevel == "" {
			t.Fatalf("summary omitted authoring dimensions: %+v", summary)
		}
	}
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"`+payload.Moments[11].ID+`"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create authored scenario: %d %s", created.Code, created.Body.String())
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
