package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/joyalzzy/playable-replays/backend/internal/engine"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const maxBodyBytes = 64 << 10

type Server struct {
	moments  map[string]model.Moment
	ordered  []model.Moment
	sessions map[string]*engine.Engine
	nextID   atomic.Uint64
	mu       sync.RWMutex
	log      *slog.Logger
}

func New(moments []model.Moment, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	indexed := make(map[string]model.Moment, len(moments))
	for _, moment := range moments {
		indexed[moment.ID] = moment
	}
	return &Server{
		moments: indexed, ordered: moments,
		sessions: make(map[string]*engine.Engine), log: logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/moments", s.listMoments)
	mux.HandleFunc("POST /api/v1/sessions", s.createSession)
	mux.HandleFunc("GET /api/v1/sessions/{id}", s.getSession)
	mux.HandleFunc("POST /api/v1/sessions/{id}/turns", s.applyTurn)
	mux.HandleFunc("POST /api/v1/sessions/{id}/reset", s.resetSession)
	return s.middleware(mux)
}

func (s *Server) listMoments(w http.ResponseWriter, _ *http.Request) {
	items := make([]model.MomentSummary, 0, len(s.ordered))
	for _, moment := range s.ordered {
		items = append(items, model.MomentSummary{
			ID: moment.ID, Slug: moment.Slug, Title: moment.Title,
			Description: moment.Description, Map: moment.Map,
			Category: moment.Authoring.Category, SkillLevel: moment.Authoring.SkillLevel,
			ReasonTags: moment.ReasonTags, Score: highlightScore(moment.Signals),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"moments": items})
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
	instance := engine.New(moment, id)
	s.mu.Lock()
	s.sessions[id] = instance
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, instance.State())
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.session(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "the requested session does not exist")
		return
	}
	writeJSON(w, http.StatusOK, instance.State())
}

func (s *Server) applyTurn(w http.ResponseWriter, r *http.Request) {
	var request model.TurnRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	instance, ok := s.sessions[r.PathValue("id")]
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "the requested session does not exist")
		return
	}
	state, err := instance.Apply(request.Action)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	instance, ok := s.sessions[r.PathValue("id")]
	if !ok {
		writeError(w, http.StatusNotFound, "session_not_found", "the requested session does not exist")
		return
	}
	writeJSON(w, http.StatusOK, instance.Reset(r.PathValue("id")))
}

func (s *Server) session(id string) (*engine.Engine, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	instance, ok := s.sessions[id]
	return instance, ok
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
	if decoder.Decode(&struct{}{}) == nil {
		return errors.New("request must contain one JSON value")
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

func highlightScore(signals model.Signals) float64 {
	score := signals.WinProbabilitySwing*0.45 +
		signals.EventDensity*0.2 +
		(1-signals.EntityProximity)*0.2 +
		signals.ResourceAsymmetry*0.15
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func StableMomentID(kind string, startSecond int) string {
	kind = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(kind), " ", "-"))
	return fmt.Sprintf("%s-%d", kind, startSecond)
}
