package telemetry

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/joyalzzy/playable-replays/backend/internal/drafts"
	"github.com/joyalzzy/playable-replays/backend/internal/fixtures"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

var (
	ErrMatchNotFound   = errors.New("telemetry match not found")
	ErrCandidateAbsent = errors.New("telemetry candidate not found")
	ErrUnauthorized    = errors.New("invalid collector token")
	ErrInvalidInput    = errors.New("invalid telemetry input")
	ErrMatchFinalized  = errors.New("telemetry match is finalized")
	ErrDraftLocked     = errors.New("candidate must be finalized before draft creation")
	ErrDraftNotFound   = errors.New("telemetry scenario draft not found")
	ErrEvidenceChanged = errors.New("detector evidence and draft identity are read-only")
	ErrStorageFailure  = errors.New("local telemetry storage failed")
	ErrTimelineAbsent  = errors.New("the identity-free timeline was not persisted")
)

const (
	maxFramesPerBatch = 16
	maxFramesPerMatch = 10_000
	maxMatches        = 32
)

type Service struct {
	mu            sync.Mutex
	nextID        int
	matches       map[string]*liveMatch
	orderedIDs    []string
	archived      map[string]model.TelemetryMatch
	archiveIDs    []string
	archiveDrafts map[string]map[string]drafts.Draft
	subscribers   map[string]map[chan model.TelemetryMatch]struct{}
	store         *localStore
}

type liveMatch struct {
	id               string
	source           string
	status           string
	token            string
	frameCount       int
	lastSecond       int
	expectedSequence int
	roster           map[string]string
	detector         Detector
	timeline         timelineBuffer
	draftStatus      map[string]string
	drafts           map[string]drafts.Draft
	savedLocally     bool
}

type DraftResult struct {
	CandidateID      string                      `json:"candidateId"`
	Status           string                      `json:"status"`
	CompletionIssues []string                    `json:"completionIssues"`
	FieldIssues      []drafts.FieldIssue         `json:"fieldIssues"`
	Acceptance       []fixtures.AcceptanceResult `json:"acceptanceResults"`
	CanPreview       bool                        `json:"canPreview"`
	CanExport        bool                        `json:"canExport"`
	Bundle           drafts.Bundle               `json:"bundle"`
}

func NewService() *Service {
	return newService(nil, persistedState{})
}

// NewPersistentService restores finalized identity-free match summaries and
// analyst drafts. Raw frames and collector tokens are intentionally not part
// of the local store contract.
func NewPersistentService(dataDirectory string, retentionDays int) (*Service, error) {
	store, err := openLocalStore(dataDirectory, retentionDays)
	if err != nil {
		return nil, err
	}
	state, err := store.load()
	if err != nil {
		return nil, err
	}
	return newService(store, state), nil
}

func newService(store *localStore, state persistedState) *Service {
	service := &Service{
		matches: map[string]*liveMatch{}, archived: map[string]model.TelemetryMatch{},
		archiveDrafts: map[string]map[string]drafts.Draft{}, subscribers: map[string]map[chan model.TelemetryMatch]struct{}{},
		store: store,
	}
	for _, envelope := range state.Matches {
		match := envelope.Match
		service.archived[match.ID] = match
		service.archiveIDs = append(service.archiveIDs, match.ID)
		if suffix, found := strings.CutPrefix(match.ID, "telemetry-match-"); found {
			if value, err := strconv.Atoi(suffix); err == nil && value > service.nextID {
				service.nextID = value
			}
		}
	}
	for matchID, byCandidate := range state.Drafts {
		service.archiveDrafts[matchID] = map[string]drafts.Draft{}
		for candidateID, envelope := range byCandidate {
			service.archiveDrafts[matchID][candidateID] = cloneDraft(envelope.Draft)
		}
		service.reconcileArchivedDraftStatus(matchID)
	}
	return service
}

func (s *Service) Start(request model.CreateTelemetryMatchRequest) (model.CreateTelemetryMatchResponse, error) {
	source := normalizeSource(request.Source)
	if !request.Consent || !validateSource(source) {
		return model.CreateTelemetryMatchResponse{}, fmt.Errorf("%w: consent is required and source must be synthetic or authorized", ErrInvalidInput)
	}
	token, err := randomToken()
	if err != nil {
		return model.CreateTelemetryMatchResponse{}, fmt.Errorf("create collector token: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.matches) >= maxMatches {
		return model.CreateTelemetryMatchResponse{}, fmt.Errorf("%w: match limit reached", ErrInvalidInput)
	}
	s.nextID++
	id := fmt.Sprintf("telemetry-match-%d", s.nextID)
	match := &liveMatch{id: id, source: source, status: "capturing", token: token, lastSecond: -1, roster: map[string]string{}, draftStatus: map[string]string{}, drafts: map[string]drafts.Draft{}}
	s.matches[id] = match
	s.orderedIDs = append(s.orderedIDs, id)
	return model.CreateTelemetryMatchResponse{Match: snapshot(match), CollectorToken: token}, nil
}

func (s *Service) List() []model.TelemetryMatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]model.TelemetryMatch, 0, len(s.orderedIDs)+len(s.archiveIDs))
	seen := make(map[string]bool, len(s.orderedIDs))
	for index := len(s.orderedIDs) - 1; index >= 0; index-- {
		id := s.orderedIDs[index]
		result = append(result, snapshot(s.matches[id]))
		seen[id] = true
	}
	for _, id := range s.archiveIDs {
		if !seen[id] {
			result = append(result, cloneTelemetryMatch(s.archived[id]))
		}
	}
	return result
}

func (s *Service) Get(id string) (model.TelemetryMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, ok := s.matches[id]
	if ok {
		return snapshot(match), nil
	}
	archived, ok := s.archived[id]
	if !ok {
		return model.TelemetryMatch{}, ErrMatchNotFound
	}
	return cloneTelemetryMatch(archived), nil
}

// Timeline returns a detached, bounded, identity-free movement/event trace.
func (s *Service) Timeline(id string) (model.TelemetryTimeline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, ok := s.matches[id]
	if !ok {
		if _, archived := s.archived[id]; archived {
			return model.TelemetryTimeline{}, ErrTimelineAbsent
		}
		return model.TelemetryTimeline{}, ErrMatchNotFound
	}
	return match.timeline.Snapshot(id), nil
}

func (s *Service) Ingest(id, token string, batch model.TelemetryFrameBatch) (model.TelemetryMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, ok := s.matches[id]
	if !ok {
		return model.TelemetryMatch{}, ErrMatchNotFound
	}
	if !tokenMatches(match.token, token) {
		return model.TelemetryMatch{}, ErrUnauthorized
	}
	if match.status != "capturing" {
		return model.TelemetryMatch{}, ErrMatchFinalized
	}
	if match.frameCount+len(batch.Frames) > maxFramesPerMatch {
		return model.TelemetryMatch{}, fmt.Errorf("%w: match exceeds %d frames", ErrInvalidInput, maxFramesPerMatch)
	}
	roster, err := validateBatch(match, batch)
	if err != nil {
		return model.TelemetryMatch{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	match.detector.Add(batch.Frames)
	match.timeline.Add(batch.Frames)
	match.roster = roster
	match.frameCount += len(batch.Frames)
	match.lastSecond = batch.Frames[len(batch.Frames)-1].Second
	match.expectedSequence++
	current := snapshot(match)
	s.broadcastLocked(id, current)
	return current, nil
}

func (s *Service) Finish(id, token string) (model.TelemetryMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, ok := s.matches[id]
	if !ok {
		return model.TelemetryMatch{}, ErrMatchNotFound
	}
	if !tokenMatches(match.token, token) {
		return model.TelemetryMatch{}, ErrUnauthorized
	}
	if match.frameCount == 0 {
		return model.TelemetryMatch{}, fmt.Errorf("%w: cannot finalize an empty match", ErrInvalidInput)
	}
	match.status = "finalized"
	current := snapshot(match)
	if s.store != nil {
		if err := s.store.saveMatch(current); err != nil {
			match.status = "capturing"
			return model.TelemetryMatch{}, fmt.Errorf("%w: save finalized match summary: %v", ErrStorageFailure, err)
		}
		match.savedLocally = true
		current = snapshot(match)
	}
	// Collector credentials are single-use and are discarded immediately after
	// finalization. They are never written to the local store.
	match.token = ""
	s.broadcastLocked(id, current)
	return current, nil
}

func (s *Service) Draft(id, candidateID string) (DraftResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, live := s.matches[id]
	if live && match.status != "finalized" {
		return DraftResult{}, ErrDraftLocked
	}
	summary, ok := s.summaryLocked(id)
	if !ok {
		return DraftResult{}, ErrMatchNotFound
	}
	candidates := summary.Candidates
	var selected *model.TelemetryCandidate
	for index := range candidates {
		if candidates[index].ID == candidateID {
			selected = &candidates[index]
			break
		}
	}
	if selected == nil {
		return DraftResult{}, ErrCandidateAbsent
	}
	data, err := json.Marshal(selected.Detection)
	if err != nil {
		return DraftResult{}, fmt.Errorf("encode detection: %w", err)
	}
	bundle, err := drafts.FromNDJSON(bytes.NewReader(append(data, '\n')))
	if err != nil {
		return DraftResult{}, fmt.Errorf("create guarded draft: %w", err)
	}
	issues := drafts.CompletionIssues(bundle.Drafts[0])
	if len(issues) == 0 {
		return DraftResult{}, errors.New("generated telemetry draft unexpectedly passed the analyst gate")
	}
	generated := bundle.Drafts[0]
	if s.store != nil {
		if err := s.store.saveDraft(id, candidateID, generated); err != nil {
			return DraftResult{}, fmt.Errorf("%w: save telemetry draft: %v", ErrStorageFailure, err)
		}
	}
	if live {
		match.drafts[candidateID] = generated
		match.draftStatus[candidateID] = "incomplete"
		current := snapshot(match)
		if err := s.persistSummaryLocked(current); err != nil {
			return DraftResult{}, err
		}
		s.broadcastLocked(id, current)
	} else {
		if s.archiveDrafts[id] == nil {
			s.archiveDrafts[id] = map[string]drafts.Draft{}
		}
		s.archiveDrafts[id][candidateID] = generated
		s.setArchivedCandidateStatus(id, candidateID, "incomplete")
		if err := s.persistSummaryLocked(s.archived[id]); err != nil {
			return DraftResult{}, err
		}
	}
	return draftResult(candidateID, generated), nil
}

// GetDraft returns a detached copy so callers cannot mutate service state.
func (s *Service) GetDraft(id, candidateID string) (DraftResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if match, ok := s.matches[id]; ok {
		draft, found := match.drafts[candidateID]
		if !found {
			return DraftResult{}, ErrDraftNotFound
		}
		return draftResult(candidateID, cloneDraft(draft)), nil
	}
	if _, ok := s.archived[id]; !ok {
		return DraftResult{}, ErrMatchNotFound
	}
	draft, ok := s.archiveDrafts[id][candidateID]
	if !ok {
		return DraftResult{}, ErrDraftNotFound
	}
	return draftResult(candidateID, cloneDraft(draft)), nil
}

// UpdateDraft stores intentionally incomplete work while protecting every
// detector-derived identity and evidence field from analyst edits.
func (s *Service) UpdateDraft(id, candidateID string, scenario model.Moment) (DraftResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, live := s.matches[id]
	var original drafts.Draft
	var ok bool
	if live {
		original, ok = match.drafts[candidateID]
	} else if _, found := s.archived[id]; found {
		original, ok = s.archiveDrafts[id][candidateID]
	} else {
		return DraftResult{}, ErrMatchNotFound
	}
	if !ok {
		return DraftResult{}, ErrDraftNotFound
	}
	// Seed is an internal int64 and may be rounded by a JavaScript JSON
	// round-trip. It is never analyst-editable, so retain the canonical value
	// before checking the visible identity and detector evidence.
	scenario.Seed = original.Scenario.Seed
	if !sameDraftIdentity(original.Scenario, scenario) {
		return DraftResult{}, ErrEvidenceChanged
	}
	updated := drafts.Draft{Status: drafts.DraftStatus, Scenario: cloneMoment(scenario)}
	result := draftResult(candidateID, updated)
	if s.store != nil {
		if err := s.store.saveDraft(id, candidateID, updated); err != nil {
			return DraftResult{}, fmt.Errorf("%w: save telemetry draft: %v", ErrStorageFailure, err)
		}
	}
	if live {
		match.drafts[candidateID] = updated
		match.draftStatus[candidateID] = result.Status
		current := snapshot(match)
		if err := s.persistSummaryLocked(current); err != nil {
			return DraftResult{}, err
		}
		s.broadcastLocked(id, current)
	} else {
		s.archiveDrafts[id][candidateID] = updated
		s.setArchivedCandidateStatus(id, candidateID, result.Status)
		if err := s.persistSummaryLocked(s.archived[id]); err != nil {
			return DraftResult{}, err
		}
	}
	return result, nil
}

func (s *Service) DraftScenario(id, candidateID string) (drafts.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if match, ok := s.matches[id]; ok {
		draft, found := match.drafts[candidateID]
		if !found {
			return drafts.Draft{}, ErrDraftNotFound
		}
		return cloneDraft(draft), nil
	}
	if _, ok := s.archived[id]; !ok {
		return drafts.Draft{}, ErrMatchNotFound
	}
	draft, ok := s.archiveDrafts[id][candidateID]
	if !ok {
		return drafts.Draft{}, ErrDraftNotFound
	}
	return cloneDraft(draft), nil
}

func (s *Service) Subscribe(id string) (<-chan model.TelemetryMatch, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	match, ok := s.matches[id]
	if !ok {
		if _, archived := s.archived[id]; archived {
			return nil, nil, ErrTimelineAbsent
		}
		return nil, nil, ErrMatchNotFound
	}
	channel := make(chan model.TelemetryMatch, 1)
	if s.subscribers[id] == nil {
		s.subscribers[id] = map[chan model.TelemetryMatch]struct{}{}
	}
	s.subscribers[id][channel] = struct{}{}
	channel <- snapshot(match)
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if listeners := s.subscribers[id]; listeners != nil {
			delete(listeners, channel)
		}
	}
	return channel, cancel, nil
}

func (s *Service) StorageStatus() (model.LocalStorageStatus, error) {
	if s.store == nil {
		return model.LocalStorageStatus{Mode: "memory-only"}, nil
	}
	status, err := s.store.status()
	if err != nil {
		return model.LocalStorageStatus{}, fmt.Errorf("%w: inspect local telemetry storage: %v", ErrStorageFailure, err)
	}
	state, err := s.store.load()
	if err != nil {
		return model.LocalStorageStatus{}, fmt.Errorf("%w: refresh retained telemetry summaries: %v", ErrStorageFailure, err)
	}
	s.syncPersistedState(state)
	return status, nil
}

func (s *Service) SetRetention(days int) (model.LocalStorageStatus, error) {
	if !validRetention(days) {
		return model.LocalStorageStatus{}, fmt.Errorf("%w: retention days must be %d..%d", ErrInvalidInput, minRetentionDays, maxRetentionDays)
	}
	if s.store == nil {
		return model.LocalStorageStatus{}, fmt.Errorf("%w: local persistence is disabled", ErrStorageFailure)
	}
	status, err := s.store.setRetention(days)
	if err != nil {
		return model.LocalStorageStatus{}, fmt.Errorf("%w: update retention: %v", ErrStorageFailure, err)
	}
	state, err := s.store.load()
	if err != nil {
		return model.LocalStorageStatus{}, fmt.Errorf("%w: refresh retained telemetry summaries: %v", ErrStorageFailure, err)
	}
	s.syncPersistedState(state)
	return status, nil
}

func (s *Service) Delete(id string) (model.DeleteLocalDataResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, live := s.matches[id]
	_, archived := s.archived[id]
	if !live && !archived {
		return model.DeleteLocalDataResponse{}, ErrMatchNotFound
	}
	deleted := model.DeleteLocalDataResponse{}
	if s.store != nil {
		result, err := s.store.deleteMatch(id)
		if err != nil {
			return model.DeleteLocalDataResponse{}, fmt.Errorf("%w: delete local match data: %v", ErrStorageFailure, err)
		}
		deleted = result
	}
	if deleted.DeletedMatches == 0 {
		deleted.DeletedMatches = 1
	}
	if draftsByCandidate := s.archiveDrafts[id]; deleted.DeletedDrafts < len(draftsByCandidate) {
		deleted.DeletedDrafts = len(draftsByCandidate)
	}
	if match := s.matches[id]; match != nil && deleted.DeletedDrafts < len(match.drafts) {
		deleted.DeletedDrafts = len(match.drafts)
	}
	s.closeSubscribersLocked(id)
	delete(s.matches, id)
	delete(s.archived, id)
	delete(s.archiveDrafts, id)
	s.orderedIDs = removeID(s.orderedIDs, id)
	s.archiveIDs = removeID(s.archiveIDs, id)
	return deleted, nil
}

func (s *Service) Purge() (model.DeleteLocalDataResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unique := map[string]bool{}
	draftCount := 0
	for id, match := range s.matches {
		unique[id] = true
		draftCount += len(match.drafts)
	}
	for id := range s.archived {
		unique[id] = true
		draftCount += len(s.archiveDrafts[id])
	}
	deleted := model.DeleteLocalDataResponse{DeletedMatches: len(unique), DeletedDrafts: draftCount}
	if s.store != nil {
		result, err := s.store.purge()
		if err != nil {
			return model.DeleteLocalDataResponse{}, fmt.Errorf("%w: delete all local telemetry data: %v", ErrStorageFailure, err)
		}
		if result.DeletedMatches > deleted.DeletedMatches {
			deleted.DeletedMatches = result.DeletedMatches
		}
		if result.DeletedDrafts > deleted.DeletedDrafts {
			deleted.DeletedDrafts = result.DeletedDrafts
		}
	}
	for id := range s.subscribers {
		s.closeSubscribersLocked(id)
	}
	s.matches = map[string]*liveMatch{}
	s.orderedIDs = nil
	s.archived = map[string]model.TelemetryMatch{}
	s.archiveIDs = nil
	s.archiveDrafts = map[string]map[string]drafts.Draft{}
	return deleted, nil
}

func ReadDocument(reader io.Reader) (model.LiveTelemetryDocument, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 16<<20))
	decoder.DisallowUnknownFields()
	var document model.LiveTelemetryDocument
	if err := decoder.Decode(&document); err != nil {
		return model.LiveTelemetryDocument{}, fmt.Errorf("decode telemetry document: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return model.LiveTelemetryDocument{}, errors.New("telemetry document must contain exactly one JSON value")
	}
	match := &liveMatch{lastSecond: -1, roster: map[string]string{}}
	if document.Version != "1.0" {
		return model.LiveTelemetryDocument{}, fmt.Errorf("unsupported telemetry version %q", document.Version)
	}
	if len(document.Frames) < 2 {
		return model.LiveTelemetryDocument{}, errors.New("telemetry document requires at least two frames")
	}
	for sequence, frame := range document.Frames {
		batch := model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: sequence, Frames: []model.LiveTelemetryFrame{frame}}
		roster, err := validateBatch(match, batch)
		if err != nil {
			return model.LiveTelemetryDocument{}, err
		}
		match.roster = roster
		match.expectedSequence++
		match.lastSecond = frame.Second
	}
	document.Frames = cloneFrames(document.Frames)
	return document, nil
}

func validateBatch(match *liveMatch, batch model.TelemetryFrameBatch) (map[string]string, error) {
	if batch.SchemaVersion != "1.0" {
		return nil, fmt.Errorf("unsupported frame schema version %q", batch.SchemaVersion)
	}
	if batch.Sequence != match.expectedSequence {
		return nil, fmt.Errorf("sequence must be %d", match.expectedSequence)
	}
	if len(batch.Frames) == 0 || len(batch.Frames) > maxFramesPerBatch {
		return nil, fmt.Errorf("frames must contain 1..%d items", maxFramesPerBatch)
	}
	lastSecond := match.lastSecond
	roster := make(map[string]string, len(match.roster))
	for id, team := range match.roster {
		roster[id] = team
	}
	for _, frame := range batch.Frames {
		if err := validateFrame(frame, lastSecond, roster); err != nil {
			return nil, err
		}
		if len(roster) == 0 {
			for _, unit := range frame.Units {
				roster[unit.ID] = unit.Team
			}
		}
		lastSecond = frame.Second
	}
	return roster, nil
}

func validateFrame(frame model.LiveTelemetryFrame, previousSecond int, roster map[string]string) error {
	if frame.Second < 0 || frame.Second <= previousSecond {
		return errors.New("frame seconds must be strictly increasing and non-negative")
	}
	if !finiteUnit(frame.WinProbability) {
		return errors.New("winProbability must be between 0 and 1")
	}
	if len(frame.Units) < 2 || len(frame.Units) > 64 {
		return errors.New("each frame must contain 2..64 units")
	}
	for _, event := range frame.Events {
		if !validateEvent(event) {
			return fmt.Errorf("unknown event type %q", event)
		}
	}
	seen, teams := map[string]bool{}, map[string]bool{}
	for _, unit := range frame.Units {
		if unit.ID == "" || unit.Team == "" || seen[unit.ID] {
			return errors.New("unit IDs and teams must be non-empty and IDs unique")
		}
		seen[unit.ID], teams[unit.Team] = true, true
		if !finiteCoordinate(unit.Position.X) || !finiteCoordinate(unit.Position.Y) || !finite(unit.MaxHP) || unit.MaxHP <= 0 ||
			!finite(unit.HP) || unit.HP < 0 || unit.HP > unit.MaxHP || !finite(unit.Gold) || unit.Gold < 0 {
			return fmt.Errorf("unit %q has invalid coordinates or resources", unit.ID)
		}
		if len(roster) > 0 && roster[unit.ID] != unit.Team {
			return errors.New("unit IDs and teams must remain stable across frames")
		}
	}
	if len(teams) != 2 || len(roster) > 0 && len(seen) != len(roster) {
		return errors.New("telemetry must contain exactly two stable teams and roster")
	}
	return nil
}

func snapshot(match *liveMatch) model.TelemetryMatch {
	status := "provisional"
	if match.status == "finalized" {
		status = "final"
	}
	candidates := match.detector.Candidates(status, match.draftStatus)
	for index := range candidates {
		candidates[index].Detection = match.timeline.AnonymizeDetection(candidates[index].Detection)
	}
	return model.TelemetryMatch{
		ID: match.id, Source: match.source, Status: match.status, FrameCount: match.frameCount,
		LastSecond: match.lastSecond, ExpectedSequence: match.expectedSequence,
		SavedLocally: match.savedLocally, TimelineAvailable: true,
		Candidates: candidates,
	}
}

func (s *Service) summaryLocked(id string) (model.TelemetryMatch, bool) {
	if match, ok := s.matches[id]; ok {
		return snapshot(match), true
	}
	match, ok := s.archived[id]
	return cloneTelemetryMatch(match), ok
}

func (s *Service) persistSummaryLocked(match model.TelemetryMatch) error {
	if s.store == nil {
		return nil
	}
	if err := s.store.saveMatch(match); err != nil {
		return fmt.Errorf("%w: save match summary: %v", ErrStorageFailure, err)
	}
	return nil
}

func (s *Service) setArchivedCandidateStatus(matchID, candidateID, status string) {
	match, ok := s.archived[matchID]
	if !ok {
		return
	}
	for index := range match.Candidates {
		if match.Candidates[index].ID == candidateID {
			match.Candidates[index].DraftStatus = status
			break
		}
	}
	s.archived[matchID] = match
}

func (s *Service) reconcileArchivedDraftStatus(matchID string) {
	for candidateID, draft := range s.archiveDrafts[matchID] {
		s.setArchivedCandidateStatus(matchID, candidateID, draftResult(candidateID, draft).Status)
	}
}

func (s *Service) syncPersistedState(state persistedState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archived = map[string]model.TelemetryMatch{}
	s.archiveIDs = nil
	s.archiveDrafts = map[string]map[string]drafts.Draft{}
	stored := map[string]bool{}
	for _, envelope := range state.Matches {
		match := envelope.Match
		stored[match.ID] = true
		if live := s.matches[match.ID]; live != nil {
			live.savedLocally = true
			continue
		}
		s.archived[match.ID] = match
		s.archiveIDs = append(s.archiveIDs, match.ID)
	}
	for _, match := range s.matches {
		if match.status == "finalized" && !stored[match.id] {
			match.savedLocally = false
		}
	}
	for matchID, byCandidate := range state.Drafts {
		if s.matches[matchID] != nil {
			continue
		}
		s.archiveDrafts[matchID] = map[string]drafts.Draft{}
		for candidateID, envelope := range byCandidate {
			s.archiveDrafts[matchID][candidateID] = cloneDraft(envelope.Draft)
		}
		s.reconcileArchivedDraftStatus(matchID)
	}
}

func (s *Service) closeSubscribersLocked(id string) {
	for channel := range s.subscribers[id] {
		close(channel)
	}
	delete(s.subscribers, id)
}

func removeID(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func cloneTelemetryMatch(value model.TelemetryMatch) model.TelemetryMatch {
	data, err := json.Marshal(value)
	if err != nil {
		panic("model.TelemetryMatch unexpectedly failed to encode: " + err.Error())
	}
	var cloned model.TelemetryMatch
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic("model.TelemetryMatch unexpectedly failed to decode: " + err.Error())
	}
	return cloned
}

func draftResult(candidateID string, draft drafts.Draft) DraftResult {
	fieldIssues := drafts.CompletionFieldIssues(draft)
	issues := make([]string, 0, len(fieldIssues))
	for _, issue := range fieldIssues {
		issues = append(issues, issue.Message)
	}
	acceptance := []fixtures.AcceptanceResult{}
	if fixtures.ValidateMoment(draft.Scenario) == nil {
		acceptance = fixtures.RunAcceptanceTests([]model.Moment{draft.Scenario})
	}
	ready := len(fieldIssues) == 0 && acceptanceCoversWinAndLoss(acceptance)
	status := "incomplete"
	if ready {
		status = "ready"
	}
	return DraftResult{
		CandidateID: candidateID, Status: status, CompletionIssues: issues,
		FieldIssues: fieldIssues, Acceptance: acceptance, CanPreview: ready, CanExport: ready,
		Bundle: drafts.Bundle{Version: drafts.BundleVersion, Drafts: []drafts.Draft{cloneDraft(draft)}},
	}
}

func acceptanceCoversWinAndLoss(results []fixtures.AcceptanceResult) bool {
	if len(results) < 2 {
		return false
	}
	for _, result := range results {
		if !result.Passed {
			return false
		}
	}
	return true
}

func sameDraftIdentity(original, updated model.Moment) bool {
	return original.ID == updated.ID && original.Slug == updated.Slug &&
		original.StartTimeSeconds == updated.StartTimeSeconds &&
		original.Authoring.Category == updated.Authoring.Category &&
		reflect.DeepEqual(original.ReasonTags, updated.ReasonTags) &&
		reflect.DeepEqual(original.Signals, updated.Signals) &&
		reflect.DeepEqual(original.SourceDetection, updated.SourceDetection)
}

func cloneDraft(value drafts.Draft) drafts.Draft {
	return drafts.Draft{Status: value.Status, Scenario: cloneMoment(value.Scenario)}
}

func cloneMoment(value model.Moment) model.Moment {
	data, err := json.Marshal(value)
	if err != nil {
		panic("model.Moment unexpectedly failed to encode: " + err.Error())
	}
	var cloned model.Moment
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic("model.Moment unexpectedly failed to decode: " + err.Error())
	}
	return cloned
}

func (s *Service) broadcastLocked(id string, value model.TelemetryMatch) {
	for channel := range s.subscribers[id] {
		select {
		case channel <- value:
		default:
			select {
			case <-channel:
			default:
			}
			channel <- value
		}
	}
}

func randomToken() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func tokenMatches(expected, supplied string) bool {
	return len(expected) == len(supplied) && subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

func finite(value float64) bool           { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func finiteUnit(value float64) bool       { return finite(value) && value >= 0 && value <= 1 }
func finiteCoordinate(value float64) bool { return finite(value) && value >= 0 && value <= 100 }
