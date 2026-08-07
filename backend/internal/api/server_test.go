package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
	"github.com/joyalzzy/playable-replays/backend/internal/positionmodel"
	"github.com/joyalzzy/playable-replays/backend/internal/telemetry"
)

type blockingModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (stub *blockingModel) NextPositions(ctx context.Context, _ engine.ModelSnapshot) (engine.ModelResult, error) {
	stub.once.Do(func() { close(stub.started) })
	select {
	case <-stub.release:
		return engine.ModelResult{ModelName: "blocking-test", ModelVersion: "1"}, nil
	case <-ctx.Done():
		return engine.ModelResult{}, ctx.Err()
	}
}

func testServer() *Server {
	moment := model.Moment{
		ID: "m1", Slug: "test", Title: "Test moment", Seed: 1, MaxTurns: 2,
		ControlledUnitID: "blue", ReasonTags: []string{"clutch"},
		MechanicBriefing: &model.MechanicBriefing{Mechanics: []model.ScenarioMechanic{
			{
				ElementID:      "test-zone",
				Name:           "Test zone",
				Description:    "A synthetic mechanic used to verify the public session contract.",
				RoleInScenario: "The controlled unit must understand this zone before acting.",
			},
		}},
		Authoring: model.ScenarioAuthoring{Category: "positioning", SkillLevel: "beginner"},
		Units: []model.Unit{
			{ID: "blue", Team: "blue", Role: "carry", Class: model.ClassMarksman, Position: model.Point{X: 30, Y: 50}, HP: 80, MaxHP: 90, Alive: true},
			{ID: "red", Team: "red", Role: "tank", Class: model.ClassTank, Position: model.Point{X: 45, Y: 50}, HP: 120, MaxHP: 160, Alive: true},
			{ID: "blue-support", Team: "blue", Role: "support", Class: model.ClassSupport, Position: model.Point{X: 25, Y: 50}, HP: 100, MaxHP: 110, Alive: true},
		},
	}
	return New([]model.Moment{moment}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return performRequest(handler, method, path, body)
}

func performRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	return result
}

func authorizedRequest(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
	if session.MechanicBriefing == nil || len(session.MechanicBriefing.Mechanics) != 1 {
		t.Fatalf("create response omitted mechanic briefing: %+v", session.MechanicBriefing)
	}
	if got := session.MechanicBriefing.Mechanics[0].ElementID; got != "test-zone" {
		t.Fatalf("create response changed mechanic element ID: %q", got)
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

func TestLiveTelemetryJourneyKeepsDraftAnalystGated(t *testing.T) {
	handler := testServer().Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches", `{"source":"synthetic","consent":true}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create telemetry match: %d %s", created.Code, created.Body.String())
	}
	var start model.CreateTelemetryMatchResponse
	if err := json.Unmarshal(created.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}

	firstBatch, err := json.Marshal(model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: 0, Frames: []model.LiveTelemetryFrame{apiTelemetryFrame(0)}})
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/frames", string(firstBatch))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected collector auth, got %d", unauthorized.Code)
	}
	for second := 0; second <= 12; second++ {
		batch, marshalErr := json.Marshal(model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: second, Frames: []model.LiveTelemetryFrame{apiTelemetryFrame(second)}})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		result := authorizedRequest(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/frames", string(batch), start.CollectorToken)
		if result.Code != http.StatusAccepted {
			t.Fatalf("ingest second %d: %d %s", second, result.Code, result.Body.String())
		}
	}
	locked := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/candidate-0-12/draft", "")
	if locked.Code != http.StatusConflict {
		t.Fatalf("capturing match should not create drafts, got %d", locked.Code)
	}
	finished := authorizedRequest(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/finish", "", start.CollectorToken)
	if finished.Code != http.StatusOK {
		t.Fatalf("finish telemetry: %d %s", finished.Code, finished.Body.String())
	}
	var match model.TelemetryMatch
	if err := json.Unmarshal(finished.Body.Bytes(), &match); err != nil {
		t.Fatal(err)
	}
	if len(match.Candidates) != 1 || match.Candidates[0].Status != "final" {
		t.Fatalf("expected one final candidate: %+v", match)
	}
	timelineResponse := request(t, handler, http.MethodGet, "/api/v1/telemetry/matches/"+match.ID+"/timeline", "")
	if timelineResponse.Code != http.StatusOK {
		t.Fatalf("get telemetry timeline: %d %s", timelineResponse.Code, timelineResponse.Body.String())
	}
	if strings.Contains(timelineResponse.Body.String(), "blue-carry") || strings.Contains(timelineResponse.Body.String(), "red-jungle") {
		t.Fatalf("timeline leaked source unit IDs: %s", timelineResponse.Body.String())
	}
	var timeline model.TelemetryTimeline
	if err := json.Unmarshal(timelineResponse.Body.Bytes(), &timeline); err != nil {
		t.Fatal(err)
	}
	if timeline.SourceFrameCount != 13 || len(timeline.Frames) != 13 || timeline.Frames[0].Units[0].TrackID != "A1" {
		t.Fatalf("unexpected visual timeline: %+v", timeline)
	}
	draft := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+match.ID+"/candidates/"+match.Candidates[0].ID+"/draft", "")
	if draft.Code != http.StatusCreated || !bytes.Contains(draft.Body.Bytes(), []byte(`"status":"incomplete"`)) || !bytes.Contains(draft.Body.Bytes(), []byte("analyst rationale is incomplete")) {
		t.Fatalf("draft was not visibly analyst-gated: %d %s", draft.Code, draft.Body.String())
	}
}

func TestTelemetryDraftWorkbenchValidatesPreviewsAndExportsWithoutPublishing(t *testing.T) {
	moments, err := fixtures.Load("../../../fixtures/moments.json")
	if err != nil {
		t.Fatal(err)
	}
	server := New(moments, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := server.Handler()
	start, match := finalizedTelemetryMatch(t, handler)
	candidate := match.Candidates[0]
	draftResponse := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/"+candidate.ID+"/draft", "")
	if draftResponse.Code != http.StatusCreated {
		t.Fatalf("create draft: %d %s", draftResponse.Code, draftResponse.Body.String())
	}
	var generated telemetry.DraftResult
	if err := json.Unmarshal(draftResponse.Body.Bytes(), &generated); err != nil {
		t.Fatal(err)
	}
	if generated.CanPreview || len(generated.FieldIssues) == 0 {
		t.Fatalf("new draft should be field-gated: %+v", generated)
	}

	completed := moments[0]
	starter := generated.Bundle.Drafts[0].Scenario
	completed.ID = starter.ID
	completed.Slug = starter.Slug
	completed.StartTimeSeconds = starter.StartTimeSeconds
	completed.Seed = int64(float64(starter.Seed)) // Simulate a browser JSON number round-trip.
	if completed.Seed == starter.Seed {
		t.Fatal("test detector seed must exceed JavaScript's safe integer precision")
	}
	completed.ReasonTags = starter.ReasonTags
	completed.Signals = starter.Signals
	completed.SourceDetection = starter.SourceDetection
	completed.Authoring.Category = starter.Authoring.Category
	updateBody, err := json.Marshal(map[string]any{"scenario": completed})
	if err != nil {
		t.Fatal(err)
	}
	updated := request(t, handler, http.MethodPut, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/"+candidate.ID+"/draft", string(updateBody))
	if updated.Code != http.StatusOK {
		t.Fatalf("complete draft: %d %s", updated.Code, updated.Body.String())
	}
	var ready telemetry.DraftResult
	if err := json.Unmarshal(updated.Body.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Status != "ready" || !ready.CanPreview || !ready.CanExport || len(ready.Acceptance) < 2 {
		t.Fatalf("completed draft did not pass the workbench gate: %+v", ready)
	}
	if ready.Bundle.Drafts[0].Scenario.Seed != starter.Seed {
		t.Fatalf("browser number rounding changed the server-owned seed")
	}

	preview := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/"+candidate.ID+"/draft/preview", "")
	if preview.Code != http.StatusCreated || !bytes.Contains(preview.Body.Bytes(), []byte(`"session"`)) {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
	}
	reviewPack := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/"+candidate.ID+"/draft/review-pack", "")
	if reviewPack.Code != http.StatusOK {
		t.Fatalf("review pack: %d %s", reviewPack.Code, reviewPack.Body.String())
	}
	var pack struct {
		Moments []model.Moment `json:"moments"`
	}
	if err := json.Unmarshal(reviewPack.Body.Bytes(), &pack); err != nil {
		t.Fatal(err)
	}
	if len(pack.Moments) != len(moments)+1 || len(server.ordered) != len(moments) {
		t.Fatalf("export should return a separate pack without changing the authored library")
	}

	tampered := completed
	tampered.SourceDetection = nil
	tamperedBody, err := json.Marshal(map[string]any{"scenario": tampered})
	if err != nil {
		t.Fatal(err)
	}
	rejected := request(t, handler, http.MethodPut, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/"+candidate.ID+"/draft", string(tamperedBody))
	if rejected.Code != http.StatusConflict {
		t.Fatalf("detector evidence edit should be rejected: %d %s", rejected.Code, rejected.Body.String())
	}
	stillReady := request(t, handler, http.MethodGet, "/api/v1/telemetry/matches/"+start.Match.ID+"/candidates/"+candidate.ID+"/draft", "")
	if stillReady.Code != http.StatusOK || !bytes.Contains(stillReady.Body.Bytes(), []byte(`"status":"ready"`)) {
		t.Fatalf("rejected evidence edit mutated the draft: %d %s", stillReady.Code, stillReady.Body.String())
	}
}

func finalizedTelemetryMatch(t *testing.T, handler http.Handler) (model.CreateTelemetryMatchResponse, model.TelemetryMatch) {
	t.Helper()
	created := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches", `{"source":"synthetic","consent":true}`)
	var start model.CreateTelemetryMatchResponse
	if err := json.Unmarshal(created.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	for second := 0; second <= 12; second++ {
		batch, err := json.Marshal(model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: second, Frames: []model.LiveTelemetryFrame{apiTelemetryFrame(second)}})
		if err != nil {
			t.Fatal(err)
		}
		response := authorizedRequest(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/frames", string(batch), start.CollectorToken)
		if response.Code != http.StatusAccepted {
			t.Fatalf("ingest second %d: %d %s", second, response.Code, response.Body.String())
		}
	}
	finished := authorizedRequest(t, handler, http.MethodPost, "/api/v1/telemetry/matches/"+start.Match.ID+"/finish", "", start.CollectorToken)
	var match model.TelemetryMatch
	if err := json.Unmarshal(finished.Body.Bytes(), &match); err != nil {
		t.Fatal(err)
	}
	return start, match
}

func apiTelemetryFrame(second int) model.LiveTelemetryFrame {
	probability := 0.75
	if second == 6 {
		probability = 0.2
	} else if second == 12 {
		probability = 0.85
	}
	return model.LiveTelemetryFrame{
		Second: second, WinProbability: probability, Events: []string{"damage", "kill"},
		Units: []model.LiveTelemetryUnit{
			{ID: "blue", Team: "blue", Position: model.Point{X: 50, Y: 50}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
			{ID: "red", Team: "red", Position: model.Point{X: 52, Y: 50}, HP: 100, MaxHP: 100, Gold: 1000, Alive: true},
		},
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

func TestClampsOverRangeMovementToClassLimit(t *testing.T) {
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
	if result.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", result.Code, result.Body.String())
	}
	current := request(t, handler, http.MethodGet, "/api/v1/sessions/"+session.ID, "")
	if err := json.Unmarshal(current.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Turn != 1 || session.Units[0].Position != (model.Point{X: 41, Y: 50}) {
		t.Fatalf("movement was not clamped to the marksman frame limit: %+v", session)
	}
}

func TestServerUsesPositionModel(t *testing.T) {
	modelRequests := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelRequests++
		var snapshot engine.ModelSnapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil ||
			snapshot.SchemaVersion != engine.PositionModelSchemaVersion || len(snapshot.Units) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"positions":[{"unitId":"red","position":{"x":100,"y":50}},{"unitId":"blue-support","position":{"x":100,"y":50}}]}`))
	}))
	defer modelServer.Close()
	connector, err := positionmodel.NewHTTPModel(modelServer.URL, "api-test", "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	base := testServer()
	server := NewWithPositionModel(base.ordered, slog.New(slog.NewTextHandler(io.Discard, nil)), connector)
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
	if modelRequests != 1 || sessionUnitPosition(session, "red") != (model.Point{X: 52, Y: 50}) ||
		sessionUnitPosition(session, "blue-support") != (model.Point{X: 33, Y: 50}) {
		t.Fatalf("position connector was not applied to opponent and teammate with class limits: %+v", session)
	}
}

func TestSlowModelDoesNotBlockOtherSessions(t *testing.T) {
	stub := &blockingModel{started: make(chan struct{}), release: make(chan struct{})}
	defer func() {
		select {
		case <-stub.release:
		default:
			close(stub.release)
		}
	}()
	base := testServer()
	server := NewWithPositionModel(base.ordered, slog.New(slog.NewTextHandler(io.Discard, nil)), stub)
	handler := server.Handler()

	first := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	second := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var firstSession, secondSession model.Session
	if err := json.Unmarshal(first.Body.Bytes(), &firstSession); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondSession); err != nil {
		t.Fatal(err)
	}

	turnDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		turnDone <- performRequest(
			handler,
			http.MethodPost,
			"/api/v1/sessions/"+firstSession.ID+"/turns",
			`{"action":{"type":"hold"}}`,
		)
	}()
	select {
	case <-stub.started:
	case <-time.After(time.Second):
		t.Fatal("position model call did not start")
	}

	readDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		readDone <- performRequest(handler, http.MethodGet, "/api/v1/sessions/"+secondSession.ID, "")
	}()
	select {
	case result := <-readDone:
		if result.Code != http.StatusOK {
			t.Fatalf("read other session: %d %s", result.Code, result.Body.String())
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("slow model call blocked an unrelated session")
	}

	close(stub.release)
	select {
	case result := <-turnDone:
		if result.Code != http.StatusOK {
			t.Fatalf("turn after model release: %d %s", result.Code, result.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("turn did not finish after model release")
	}
}

func sessionUnitPosition(session model.Session, id string) model.Point {
	for _, unit := range session.Units {
		if unit.ID == id {
			return unit.Position
		}
	}
	return model.Point{X: -1, Y: -1}
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

func TestRouterReturnsStructured404And405(t *testing.T) {
	handler := testServer().Handler()
	for _, test := range []struct {
		method string
		path   string
		status int
		code   string
	}{
		{method: http.MethodGet, path: "/api/v1/does-not-exist", status: http.StatusNotFound, code: "route_not_found"},
		{method: http.MethodGet, path: "/api/v1/moments/", status: http.StatusNotFound, code: "route_not_found"},
		{method: http.MethodPost, path: "/api/v1/moments", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
	} {
		result := request(t, handler, test.method, test.path, "")
		var response model.ErrorResponse
		if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
			t.Fatalf("%s %s returned non-JSON: %d %q", test.method, test.path, result.Code, result.Body.String())
		}
		if result.Code != test.status || response.Error.Code != test.code {
			t.Fatalf("%s %s: got %d/%q, want %d/%q", test.method, test.path, result.Code, response.Error.Code, test.status, test.code)
		}
	}
	wrongMethod := request(t, handler, http.MethodPost, "/api/v1/moments", "")
	if wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("405 omitted a precise Allow header: %q", wrongMethod.Header().Get("Allow"))
	}
}

func TestDecodeJSONRejectsAnyTrailingData(t *testing.T) {
	handler := testServer().Handler()
	for _, body := range []string{
		`{"momentId":"m1"}{"momentId":"m1"}`,
		`{"momentId":"m1"} trailing`,
	} {
		result := request(t, handler, http.MethodPost, "/api/v1/sessions", body)
		if result.Code != http.StatusBadRequest || !bytes.Contains(result.Body.Bytes(), []byte(`"code":"invalid_request"`)) {
			t.Fatalf("trailing JSON was accepted: %d %s", result.Code, result.Body.String())
		}
	}
}

func TestSessionMutationsAreRateLimitedAndReadsRemainAvailable(t *testing.T) {
	server := testServer()
	handler := server.Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < sessionMutationLimit; attempt++ {
		result := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/reset", "")
		if result.Code != http.StatusOK {
			t.Fatalf("mutation %d was limited early: %d %s", attempt, result.Code, result.Body.String())
		}
	}
	limited := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/reset", "")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" || !bytes.Contains(limited.Body.Bytes(), []byte(`"code":"rate_limited"`)) {
		t.Fatalf("expected structured 429 response, got %d %s", limited.Code, limited.Body.String())
	}
	read := request(t, handler, http.MethodGet, "/api/v1/sessions/"+session.ID, "")
	if read.Code != http.StatusOK {
		t.Fatalf("read should remain available after a mutation limit: %d %s", read.Code, read.Body.String())
	}
}

func TestSessionReadsAndMutationsUsePerSessionLock(t *testing.T) {
	handler := testServer().Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(2)
		go func() {
			defer wait.Done()
			result := request(t, handler, http.MethodGet, "/api/v1/sessions/"+session.ID, "")
			if result.Code != http.StatusOK {
				t.Errorf("concurrent read: %d %s", result.Code, result.Body.String())
			}
		}()
		go func() {
			defer wait.Done()
			result := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/reset", "")
			if result.Code != http.StatusOK && result.Code != http.StatusTooManyRequests {
				t.Errorf("concurrent mutation: %d %s", result.Code, result.Body.String())
			}
		}()
	}
	wait.Wait()
}

func TestPersistentStorageAPIUpdatesRetentionAndDeletesData(t *testing.T) {
	service, err := telemetry.NewPersistentService(t.TempDir(), 7)
	if err != nil {
		t.Fatal(err)
	}
	server := testServer()
	server.telemetry = service
	handler := server.Handler()
	status := request(t, handler, http.MethodGet, "/api/v1/local-storage", "")
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"retentionDays":7`)) {
		t.Fatalf("get storage status: %d %s", status.Code, status.Body.String())
	}
	updated := request(t, handler, http.MethodPut, "/api/v1/local-storage/retention", `{"retentionDays":30}`)
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte(`"retentionDays":30`)) {
		t.Fatalf("update retention: %d %s", updated.Code, updated.Body.String())
	}
	invalid := request(t, handler, http.MethodPut, "/api/v1/local-storage/retention", `{"retentionDays":0}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid retention should be rejected: %d %s", invalid.Code, invalid.Body.String())
	}
	created := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches", `{"source":"synthetic","consent":true}`)
	var started model.CreateTelemetryMatchResponse
	if err := json.Unmarshal(created.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	deleted := request(t, handler, http.MethodDelete, "/api/v1/telemetry/matches/"+started.Match.ID, "")
	if deleted.Code != http.StatusOK || !bytes.Contains(deleted.Body.Bytes(), []byte(`"deletedMatches":1`)) {
		t.Fatalf("delete telemetry match: %d %s", deleted.Code, deleted.Body.String())
	}
	missing := request(t, handler, http.MethodGet, "/api/v1/telemetry/matches/"+started.Match.ID, "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted telemetry match remains available: %d %s", missing.Code, missing.Body.String())
	}
	for index := 0; index < 2; index++ {
		result := request(t, handler, http.MethodPost, "/api/v1/telemetry/matches", `{"source":"synthetic","consent":true}`)
		if result.Code != http.StatusCreated {
			t.Fatalf("create telemetry match %d: %d %s", index, result.Code, result.Body.String())
		}
	}
	purged := request(t, handler, http.MethodDelete, "/api/v1/telemetry/matches", "")
	if purged.Code != http.StatusOK || !bytes.Contains(purged.Body.Bytes(), []byte(`"deletedMatches":2`)) {
		t.Fatalf("delete all telemetry: %d %s", purged.Code, purged.Body.String())
	}
	listed := request(t, handler, http.MethodGet, "/api/v1/telemetry/matches", "")
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"matches":[]`)) {
		t.Fatalf("delete all left telemetry behind: %d %s", listed.Code, listed.Body.String())
	}
}

func TestStableMomentIDIncludesWindow(t *testing.T) {
	if StableMomentID("Team Fight", 42) == StableMomentID("Team Fight", 43) {
		t.Fatal("moment IDs from distinct windows collided")
	}
}
