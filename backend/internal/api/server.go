package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/drafts"
	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/highlight"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
	"github.com/joyalzzy/playable-replays/backend/internal/telemetry"
)

const maxBodyBytes = 64 << 10

type Server struct {
	moments       map[string]model.Moment
	ordered       []model.Moment
	sessions      map[string]*sessionEntry
	positionModel engine.PositionModel
	nextID        atomic.Uint64
	mu            sync.RWMutex
	log           *slog.Logger
	telemetry     *telemetry.Service
	now           func() time.Time
}

type sessionEntry struct {
	mu      sync.RWMutex
	engine  *engine.Engine
	limiter fixedWindowLimiter
}

func New(moments []model.Moment, logger *slog.Logger) *Server {
	return NewWithTelemetryAndPositionModel(moments, logger, telemetry.NewService(), nil)
}

func NewWithTelemetry(moments []model.Moment, logger *slog.Logger, telemetryService *telemetry.Service) *Server {
	return NewWithTelemetryAndPositionModel(moments, logger, telemetryService, nil)
}

func NewWithPositionModel(moments []model.Moment, logger *slog.Logger, positionModel engine.PositionModel) *Server {
	return NewWithTelemetryAndPositionModel(moments, logger, telemetry.NewService(), positionModel)
}

func NewWithTelemetryAndPositionModel(
	moments []model.Moment,
	logger *slog.Logger,
	telemetryService *telemetry.Service,
	positionModel engine.PositionModel,
) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if telemetryService == nil {
		telemetryService = telemetry.NewService()
	}
	indexed := make(map[string]model.Moment, len(moments))
	for _, moment := range moments {
		indexed[moment.ID] = moment
	}
	return &Server{
		moments: indexed, ordered: moments,
		sessions: make(map[string]*sessionEntry), log: logger,
		positionModel: positionModel, telemetry: telemetryService, now: time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	router := &structuredRouter{}
	router.handle(http.MethodGet, "/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.handle(http.MethodGet, "/api/v1/moments", s.listMoments)
	router.handle(http.MethodPost, "/api/v1/sessions", s.createSession)
	router.handle(http.MethodGet, "/api/v1/sessions/{id}", s.getSession)
	router.handle(http.MethodPost, "/api/v1/sessions/{id}/turns", s.applyTurn)
	router.handle(http.MethodPost, "/api/v1/sessions/{id}/reset", s.resetSession)
	router.handle(http.MethodGet, "/api/v1/telemetry/matches", s.listTelemetryMatches)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches", s.createTelemetryMatch)
	router.handle(http.MethodDelete, "/api/v1/telemetry/matches", s.deleteAllTelemetryMatches)
	router.handle(http.MethodGet, "/api/v1/telemetry/matches/{id}", s.getTelemetryMatch)
	router.handle(http.MethodDelete, "/api/v1/telemetry/matches/{id}", s.deleteTelemetryMatch)
	router.handle(http.MethodGet, "/api/v1/telemetry/matches/{id}/timeline", s.getTelemetryTimeline)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches/{id}/frames", s.ingestTelemetryFrames)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches/{id}/finish", s.finishTelemetryMatch)
	router.handle(http.MethodGet, "/api/v1/telemetry/matches/{id}/events", s.streamTelemetryMatch)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft", s.createTelemetryDraft)
	router.handle(http.MethodGet, "/api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft", s.getTelemetryDraft)
	router.handle(http.MethodPut, "/api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft", s.updateTelemetryDraft)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft/validate", s.validateTelemetryDraft)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft/preview", s.previewTelemetryDraft)
	router.handle(http.MethodPost, "/api/v1/telemetry/matches/{id}/candidates/{candidateId}/draft/review-pack", s.exportTelemetryDraft)
	router.handle(http.MethodGet, "/api/v1/local-storage", s.getLocalStorage)
	router.handle(http.MethodPut, "/api/v1/local-storage/retention", s.updateLocalStorageRetention)
	return s.middleware(router)
}

func (s *Server) listTelemetryMatches(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"matches": s.telemetry.List()})
}

func (s *Server) getLocalStorage(w http.ResponseWriter, _ *http.Request) {
	status, err := s.telemetry.StorageStatus()
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) updateLocalStorageRetention(w http.ResponseWriter, r *http.Request) {
	var request model.UpdateRetentionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	status, err := s.telemetry.SetRetention(request.RetentionDays)
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) deleteTelemetryMatch(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.telemetry.Delete(r.PathValue("id"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (s *Server) deleteAllTelemetryMatches(w http.ResponseWriter, _ *http.Request) {
	deleted, err := s.telemetry.Purge()
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (s *Server) createTelemetryMatch(w http.ResponseWriter, r *http.Request) {
	var request model.CreateTelemetryMatchRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	created, err := s.telemetry.Start(request)
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getTelemetryMatch(w http.ResponseWriter, r *http.Request) {
	match, err := s.telemetry.Get(r.PathValue("id"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, match)
}

func (s *Server) getTelemetryTimeline(w http.ResponseWriter, r *http.Request) {
	timeline, err := s.telemetry.Timeline(r.PathValue("id"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func (s *Server) ingestTelemetryFrames(w http.ResponseWriter, r *http.Request) {
	var batch model.TelemetryFrameBatch
	if err := decodeJSON(w, r, &batch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	match, err := s.telemetry.Ingest(r.PathValue("id"), bearerToken(r), batch)
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, match)
}

func (s *Server) finishTelemetryMatch(w http.ResponseWriter, r *http.Request) {
	match, err := s.telemetry.Finish(r.PathValue("id"), bearerToken(r))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, match)
}

func (s *Server) createTelemetryDraft(w http.ResponseWriter, r *http.Request) {
	result, err := s.telemetry.Draft(r.PathValue("id"), r.PathValue("candidateId"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getTelemetryDraft(w http.ResponseWriter, r *http.Request) {
	result, err := s.telemetry.GetDraft(r.PathValue("id"), r.PathValue("candidateId"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) updateTelemetryDraft(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Scenario model.Moment `json:"scenario"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := s.telemetry.UpdateDraft(r.PathValue("id"), r.PathValue("candidateId"), request.Scenario)
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) validateTelemetryDraft(w http.ResponseWriter, r *http.Request) {
	result, err := s.telemetry.GetDraft(r.PathValue("id"), r.PathValue("candidateId"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) previewTelemetryDraft(w http.ResponseWriter, r *http.Request) {
	draft, err := s.telemetry.DraftScenario(r.PathValue("id"), r.PathValue("candidateId"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	if err := drafts.ValidateDraft(draft); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "draft_incomplete", err.Error())
		return
	}
	id := "preview-" + strconv.FormatUint(s.nextID.Add(1), 36)
	instance := engine.NewWithPositionModel(draft.Scenario, id, s.positionModel)
	s.mu.Lock()
	s.sessions[id] = &sessionEntry{engine: instance}
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"moment":  momentSummary(draft.Scenario),
		"session": instance.State(),
	})
}

func (s *Server) exportTelemetryDraft(w http.ResponseWriter, r *http.Request) {
	draft, err := s.telemetry.DraftScenario(r.PathValue("id"), r.PathValue("candidateId"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	moments, err := drafts.PreparePack(draft, s.ordered)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "draft_incomplete", err.Error())
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", draft.Scenario.Slug+"-review-pack.json"))
	writeJSON(w, http.StatusOK, map[string]any{"version": drafts.BundleVersion, "moments": moments})
}

func (s *Server) streamTelemetryMatch(w http.ResponseWriter, r *http.Request) {
	updates, cancel, err := s.telemetry.Subscribe(r.PathValue("id"))
	if err != nil {
		writeTelemetryError(w, err)
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream_unavailable", "live updates are unavailable")
		return
	}
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case match, open := <-updates:
			if !open {
				return
			}
			data, encodeErr := json.Marshal(match)
			if encodeErr != nil {
				return
			}
			if _, writeErr := fmt.Fprintf(w, "event: match\ndata: %s\n\n", data); writeErr != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, writeErr := fmt.Fprint(w, ": keep-alive\n\n"); writeErr != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) listMoments(w http.ResponseWriter, _ *http.Request) {
	items := make([]model.MomentSummary, 0, len(s.ordered))
	for _, moment := range s.ordered {
		items = append(items, momentSummary(moment))
	}
	writeJSON(w, http.StatusOK, map[string]any{"moments": items})
}

func momentSummary(moment model.Moment) model.MomentSummary {
	return model.MomentSummary{
		ID: moment.ID, Slug: moment.Slug, Title: moment.Title,
		Description: moment.Description, Map: moment.Map,
		Category: moment.Authoring.Category, SkillLevel: moment.Authoring.SkillLevel,
		ReasonTags: moment.ReasonTags, Score: highlight.Score(moment.Signals),
	}
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var request model.CreateSessionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	moment, ok := s.moments[request.MomentID]
	if !ok {
		writeError(w, http.StatusNotFound, "moment_not_found", "the requested moment does not exist")
		return
	}
	id := "session-" + strconv.FormatUint(s.nextID.Add(1), 36)
	entry := &sessionEntry{engine: engine.NewWithPositionModel(moment, id, s.positionModel)}
	state := entry.engine.State()
	s.mu.Lock()
	s.sessions[id] = entry
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, state)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	entry, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "the requested session does not exist")
		return
	}
	entry.mu.RLock()
	state := entry.engine.State()
	entry.mu.RUnlock()
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) applyTurn(w http.ResponseWriter, r *http.Request) {
	var request model.TurnRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	entry, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "the requested session does not exist")
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !s.allowSessionMutation(w, entry) {
		return
	}
	state, err := entry.engine.ApplyContext(r.Context(), request.Action)
	if errors.Is(err, engine.ErrIllegalAction) {
		writeError(w, http.StatusUnprocessableEntity, "illegal_action", err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "simulation_error", "the simulator could not resolve the turn")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) resetSession(w http.ResponseWriter, r *http.Request) {
	entry, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "the requested session does not exist")
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !s.allowSessionMutation(w, entry) {
		return
	}
	writeJSON(w, http.StatusOK, entry.engine.Reset(r.PathValue("id")))
}

func (s *Server) session(id string) (*sessionEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	instance, ok := s.sessions[id]
	return instance, ok
}

func (s *Server) allowSessionMutation(w http.ResponseWriter, entry *sessionEntry) bool {
	allowed, retry := entry.limiter.allow(s.now(), sessionMutationLimit, sessionMutationWindow)
	if allowed {
		return true
	}
	seconds := int(retry.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many session mutations; retry after the indicated delay")
	return false
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if origin := r.Header.Get("Origin"); origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	scheme, token, found := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func writeTelemetryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, telemetry.ErrMatchNotFound), errors.Is(err, telemetry.ErrCandidateAbsent), errors.Is(err, telemetry.ErrDraftNotFound):
		writeError(w, http.StatusNotFound, "telemetry_not_found", err.Error())
	case errors.Is(err, telemetry.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "collector_unauthorized", "the collector token is missing or invalid")
	case errors.Is(err, telemetry.ErrTimelineAbsent):
		writeError(w, http.StatusGone, "telemetry_timeline_unavailable", "the identity-free timeline was memory-only and expired when the service stopped")
	case errors.Is(err, telemetry.ErrMatchFinalized), errors.Is(err, telemetry.ErrDraftLocked):
		writeError(w, http.StatusConflict, "telemetry_state_conflict", err.Error())
	case errors.Is(err, telemetry.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_telemetry", err.Error())
	case errors.Is(err, telemetry.ErrEvidenceChanged):
		writeError(w, http.StatusConflict, "draft_evidence_locked", err.Error())
	case errors.Is(err, telemetry.ErrStorageFailure):
		writeError(w, http.StatusInternalServerError, "local_storage_error", "the local summary store could not complete the operation")
	default:
		writeError(w, http.StatusInternalServerError, "telemetry_error", "the telemetry operation could not be completed")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request must contain exactly one JSON value")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var response model.ErrorResponse
	response.Error.Code = code
	response.Error.Message = message
	writeJSON(w, status, response)
}

func StableMomentID(kind string, startSecond int) string {
	kind = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(kind), " ", "-"))
	return fmt.Sprintf("%s-%d", kind, startSecond)
}
