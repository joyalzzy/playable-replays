package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/highlight"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const maxBodyBytes = 64 << 10

type Server struct {
	moments  map[string]model.Moment
	ordered  []model.Moment
	sessions map[string]*sessionEntry
	botModel engine.BotModel
	nextID   atomic.Uint64
	mu       sync.RWMutex
	log      *slog.Logger
	now      func() time.Time
}

type sessionEntry struct {
	mu      sync.RWMutex
	engine  *engine.Engine
	limiter fixedWindowLimiter
}

func New(moments []model.Moment, logger *slog.Logger) *Server {
	return NewWithBotModel(moments, logger, nil)
}

func NewWithBotModel(moments []model.Moment, logger *slog.Logger, botModel engine.BotModel) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	indexed := make(map[string]model.Moment, len(moments))
	for _, moment := range moments {
		indexed[moment.ID] = moment
	}
	return &Server{
		moments: indexed, ordered: moments,
		sessions: make(map[string]*sessionEntry), log: logger,
		botModel: botModel, now: time.Now,
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
	router.handle(http.MethodPost, "/api/v1/sessions/{id}/dodge", s.dodge)
	router.handle(http.MethodPost, "/api/v1/sessions/{id}/reset", s.resetSession)
	return s.middleware(router)
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
	entry := &sessionEntry{engine: engine.NewWithBotModel(moment, id, s.botModel)}
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

func (s *Server) dodge(w http.ResponseWriter, r *http.Request) {
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
	state, err := entry.engine.Dodge()
	if errors.Is(err, engine.ErrDodgeUnavailable) {
		writeError(w, http.StatusUnprocessableEntity, "dodge_unavailable", "no incoming projectile can be dodged or no Dodge charges remain")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "simulation_error", "the simulator could not resolve the Dodge reaction")
		return
	}
	writeJSON(w, http.StatusOK, state)
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
