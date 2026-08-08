package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
	"github.com/joyalzzy/playable-replays/backend/internal/positionmodel"
)

type blockingModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (stub *blockingModel) NextActions(ctx context.Context, snapshot engine.BotSnapshot) (engine.BotModelResult, error) {
	stub.once.Do(func() { close(stub.started) })
	select {
	case <-stub.release:
		actions := make([]engine.BotActionSuggestion, 0, len(snapshot.Units))
		for _, unit := range snapshot.Units {
			if unit.Alive && unit.ID != snapshot.ControlledUnitID {
				actions = append(actions, engine.BotActionSuggestion{
					UnitID: unit.ID, Action: model.Action{Type: "hold"},
				})
			}
		}
		return engine.BotModelResult{ModelName: "blocking-test", ModelVersion: "2", Actions: actions}, nil
	case <-ctx.Done():
		return engine.BotModelResult{}, ctx.Err()
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
	if len(session.LegalActions) != 4 || session.LegalActions[0] != "move" || session.LegalActions[3] != "retreat" {
		t.Fatalf("expected the four tactical actions, got %v", session.LegalActions)
	}
	if len(session.Turrets) != 6 || len(session.Projectiles) != 0 || session.ProjectileCharges != 2 ||
		!session.ProjectileAvailable || session.DodgeCharges != 2 ||
		session.DodgeAvailable || session.BotControl.Source != "deterministic-fallback" {
		t.Fatalf("create response omitted the map, Dodge, or bot-control contract: %+v", session)
	}
	turn := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/turns", `{"action":{"type":"contest"},"targetUnitId":"red"}`)
	if turn.Code != http.StatusOK {
		t.Fatalf("turn: %d %s", turn.Code, turn.Body.String())
	}
	reset := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/reset", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset: %d", reset.Code)
	}
}

func TestFireProjectileEndpointDoesNotAdvanceTurn(t *testing.T) {
	handler := testServer().Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	fired := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/fire", `{"sourceUnitId":"blue","targetUnitId":"red"}`)
	if fired.Code != http.StatusOK {
		t.Fatalf("fire projectile: %d %s", fired.Code, fired.Body.String())
	}
	if err := json.Unmarshal(fired.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Turn != 0 || session.ProjectileCharges != 1 || session.ProjectileAvailable ||
		len(session.Projectiles) != 1 || session.Projectiles[0].SourceUnitID != "blue" ||
		session.Projectiles[0].TargetUnitID != "red" {
		t.Fatalf("fire endpoint did not queue the bounded reaction: %+v", session)
	}

	unavailable := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/fire", `{"sourceUnitId":"blue","targetUnitId":"red"}`)
	if unavailable.Code != http.StatusUnprocessableEntity ||
		!bytes.Contains(unavailable.Body.Bytes(), []byte(`"code":"projectile_unavailable"`)) {
		t.Fatalf("expected structured unavailable-projectile response, got %d %s", unavailable.Code, unavailable.Body.String())
	}
}

func TestTargetedContestRejectsOutOfRangeEnemyWithoutMutation(t *testing.T) {
	base := testServer()
	moment := base.ordered[0]
	moment.Units[0].VisionRange = 100
	moment.Units[1].Position = model.Point{X: 70, Y: 50}
	handler := New([]model.Moment{moment}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var before model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	result := request(t, handler, http.MethodPost, "/api/v1/sessions/"+before.ID+"/turns", `{"action":{"type":"contest"},"targetUnitId":"red"}`)
	if result.Code != http.StatusUnprocessableEntity ||
		!bytes.Contains(result.Body.Bytes(), []byte(`"code":"illegal_action"`)) {
		t.Fatalf("expected out-of-range contest rejection, got %d %s", result.Code, result.Body.String())
	}
	current := request(t, handler, http.MethodGet, "/api/v1/sessions/"+before.ID, "")
	var after model.Session
	if err := json.Unmarshal(current.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected targeted contest mutated session state")
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
	if listed.Code != http.StatusOK || len(payload.Moments) != 3 {
		t.Fatalf("expected three authored summaries, got %d with status %d", len(payload.Moments), listed.Code)
	}
	for _, summary := range payload.Moments {
		if summary.Category == "" || summary.SkillLevel == "" {
			t.Fatalf("summary omitted authoring dimensions: %+v", summary)
		}
	}
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"`+payload.Moments[len(payload.Moments)-1].ID+`"}`)
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

func TestDodgeEndpointEvadesIncomingProjectileWithoutAdvancingTurn(t *testing.T) {
	base := testServer()
	moment := base.ordered[0]
	moment.Units[1] = model.Unit{
		ID: "red", Team: "red", Role: "marksman", Class: model.ClassMarksman, Policy: "aggressive",
		Position: model.Point{X: 45, Y: 50}, HP: 90, MaxHP: 90,
		AttackDamage: 20, AttackCooldown: 1, Armor: 12, VisionRange: 34, Alive: true,
	}
	handler := New([]model.Moment{moment}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	created := request(t, handler, http.MethodPost, "/api/v1/sessions", `{"momentId":"m1"}`)
	var session model.Session
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	turn := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/turns", `{"action":{"type":"hold"}}`)
	if turn.Code != http.StatusOK {
		t.Fatalf("create projectile: %d %s", turn.Code, turn.Body.String())
	}
	if err := json.Unmarshal(turn.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Turn != 1 || len(session.Projectiles) != 1 || !session.DodgeAvailable || session.DodgeCharges != 2 {
		t.Fatalf("expected an incoming projectile before Dodge: %+v", session)
	}

	dodged := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/dodge", "")
	if dodged.Code != http.StatusOK {
		t.Fatalf("Dodge: %d %s", dodged.Code, dodged.Body.String())
	}
	if err := json.Unmarshal(dodged.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.Turn != 1 || len(session.Projectiles) != 0 || session.DodgeAvailable || session.DodgeCharges != 1 {
		t.Fatalf("Dodge should consume one projectile and charge without advancing the tactical turn: %+v", session)
	}

	unavailable := request(t, handler, http.MethodPost, "/api/v1/sessions/"+session.ID+"/dodge", "")
	if unavailable.Code != http.StatusUnprocessableEntity ||
		!bytes.Contains(unavailable.Body.Bytes(), []byte(`"code":"dodge_unavailable"`)) {
		t.Fatalf("expected structured unavailable-Dodge response, got %d %s", unavailable.Code, unavailable.Body.String())
	}
}

func TestServerUsesBotModel(t *testing.T) {
	modelRequests := 0
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelRequests++
		var snapshot engine.BotSnapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil ||
			snapshot.SchemaVersion != engine.BotModelSchemaVersion || len(snapshot.Units) != 3 ||
			len(snapshot.LegalActions) != 4 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"actions":[{"unitId":"red","action":{"type":"move","target":{"x":100,"y":50}}},{"unitId":"blue-support","action":{"type":"move","target":{"x":100,"y":50}}}]}`))
	}))
	defer modelServer.Close()
	connector, err := positionmodel.NewHTTPModel(modelServer.URL, "api-test", "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	base := testServer()
	server := NewWithBotModel(base.ordered, slog.New(slog.NewTextHandler(io.Discard, nil)), connector)
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
		t.Fatalf("bot connector actions were not applied with authoritative class limits: %+v", session)
	}
	if session.BotControl.Source != "external-model" || session.BotControl.ModelName != "api-test" ||
		session.BotControl.ModelVersion != "1" {
		t.Fatalf("bot model provenance was not serialized: %+v", session.BotControl)
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
	server := NewWithBotModel(base.ordered, slog.New(slog.NewTextHandler(io.Discard, nil)), stub)
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
		t.Fatal("bot model call did not start")
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

func TestRemovedTelemetryRoutesReturnStructured404(t *testing.T) {
	handler := testServer().Handler()
	for _, path := range []string{
		"/api/v1/telemetry/matches",
		"/api/v1/telemetry/matches/example",
		"/api/v1/local-storage",
	} {
		result := request(t, handler, http.MethodGet, path, "")
		var response model.ErrorResponse
		if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
			t.Fatalf("GET %s returned non-JSON: %d %q", path, result.Code, result.Body.String())
		}
		if result.Code != http.StatusNotFound || response.Error.Code != "route_not_found" {
			t.Fatalf("GET %s: got %d/%q, want 404/route_not_found", path, result.Code, response.Error.Code)
		}
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
