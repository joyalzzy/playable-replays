package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/drafts"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

const (
	localStoreSchemaVersion = "1.0"
	defaultRetentionDays    = 7
	minRetentionDays        = 1
	maxRetentionDays        = 365
)

type storedMatchEnvelope struct {
	SchemaVersion string               `json:"schemaVersion"`
	SavedAt       time.Time            `json:"savedAt"`
	Match         model.TelemetryMatch `json:"match"`
}

type storedDraftEnvelope struct {
	SchemaVersion string       `json:"schemaVersion"`
	SavedAt       time.Time    `json:"savedAt"`
	Draft         drafts.Draft `json:"draft"`
}

type localStoreSettings struct {
	SchemaVersion string `json:"schemaVersion"`
	RetentionDays int    `json:"retentionDays"`
}

type persistedState struct {
	Matches []storedMatchEnvelope
	Drafts  map[string]map[string]storedDraftEnvelope
}

// localStore persists only finalized summaries and analyst-authored drafts.
// Its public methods cannot accept collector tokens or raw telemetry frames.
type localStore struct {
	mu            sync.Mutex
	root          string
	matchesDir    string
	draftsDir     string
	settingsPath  string
	retentionDays int
	now           func() time.Time
}

func openLocalStore(root string, configuredRetention int) (*localStore, error) {
	return openLocalStoreWithClock(root, configuredRetention, time.Now)
}

func openLocalStoreWithClock(root string, configuredRetention int, now func() time.Time) (*localStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local data directory is required")
	}
	if configuredRetention == 0 {
		configuredRetention = defaultRetentionDays
	}
	if !validRetention(configuredRetention) {
		return nil, fmt.Errorf("retention days must be %d..%d", minRetentionDays, maxRetentionDays)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local data directory: %w", err)
	}
	store := &localStore{
		root: absolute, matchesDir: filepath.Join(absolute, "matches"),
		draftsDir: filepath.Join(absolute, "drafts"), settingsPath: filepath.Join(absolute, "settings.json"),
		retentionDays: configuredRetention, now: now,
	}
	for _, directory := range []string{store.root, store.matchesDir, store.draftsDir} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return nil, fmt.Errorf("create local data directory: %w", err)
		}
	}
	if err := store.loadSettings(); err != nil {
		return nil, err
	}
	if _, err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *localStore) loadSettings() error {
	var settings localStoreSettings
	if err := decodeStrictFile(s.settingsPath, &settings); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.writeSettings()
		}
		return fmt.Errorf("read local storage settings: %w", err)
	}
	if settings.SchemaVersion != localStoreSchemaVersion || !validRetention(settings.RetentionDays) {
		return errors.New("local storage settings are invalid")
	}
	s.retentionDays = settings.RetentionDays
	return nil
}

func (s *localStore) load() (persistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *localStore) loadLocked() (persistedState, error) {
	if err := s.cleanupExpiredLocked(); err != nil {
		return persistedState{}, err
	}
	state := persistedState{Drafts: map[string]map[string]storedDraftEnvelope{}}
	entries, err := os.ReadDir(s.matchesDir)
	if err != nil {
		return persistedState{}, fmt.Errorf("list local match summaries: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var envelope storedMatchEnvelope
		if err := decodeStrictFile(filepath.Join(s.matchesDir, entry.Name()), &envelope); err != nil {
			return persistedState{}, fmt.Errorf("read local match summary %q: %w", entry.Name(), err)
		}
		if err := validateStoredMatch(envelope); err != nil {
			return persistedState{}, fmt.Errorf("validate local match summary %q: %w", entry.Name(), err)
		}
		envelope.Match.SavedLocally = true
		envelope.Match.TimelineAvailable = false
		state.Matches = append(state.Matches, envelope)
	}
	sort.Slice(state.Matches, func(i, j int) bool { return state.Matches[i].SavedAt.After(state.Matches[j].SavedAt) })
	for _, match := range state.Matches {
		directory := filepath.Join(s.draftsDir, match.Match.ID)
		if err := validateExistingDirectory(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
			return persistedState{}, fmt.Errorf("validate local draft directory for %q: %w", match.Match.ID, err)
		}
		draftEntries, readErr := os.ReadDir(directory)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return persistedState{}, fmt.Errorf("list local drafts for %q: %w", match.Match.ID, readErr)
		}
		for _, entry := range draftEntries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			candidateID := strings.TrimSuffix(entry.Name(), ".json")
			if !safeStoreID(candidateID) {
				return persistedState{}, fmt.Errorf("unsafe local draft filename %q", entry.Name())
			}
			var envelope storedDraftEnvelope
			if err := decodeStrictFile(filepath.Join(directory, entry.Name()), &envelope); err != nil {
				return persistedState{}, fmt.Errorf("read local draft %q: %w", entry.Name(), err)
			}
			if envelope.SchemaVersion != localStoreSchemaVersion || envelope.SavedAt.IsZero() || envelope.Draft.Status != drafts.DraftStatus {
				return persistedState{}, fmt.Errorf("local draft %q has an invalid envelope", entry.Name())
			}
			if state.Drafts[match.Match.ID] == nil {
				state.Drafts[match.Match.ID] = map[string]storedDraftEnvelope{}
			}
			state.Drafts[match.Match.ID][candidateID] = envelope
		}
	}
	return state, nil
}

func (s *localStore) saveMatch(match model.TelemetryMatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !safeStoreID(match.ID) || match.Status != "finalized" {
		return errors.New("only finalized generated match summaries can be saved")
	}
	match.SavedLocally = true
	match.TimelineAvailable = false
	envelope := storedMatchEnvelope{SchemaVersion: localStoreSchemaVersion, SavedAt: s.now().UTC(), Match: match}
	return writeJSONAtomic(filepath.Join(s.matchesDir, match.ID+".json"), envelope)
}

func (s *localStore) saveDraft(matchID, candidateID string, draft drafts.Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !safeStoreID(matchID) || !safeStoreID(candidateID) || draft.Status != drafts.DraftStatus {
		return errors.New("invalid local draft identity")
	}
	directory := filepath.Join(s.draftsDir, matchID)
	if err := ensurePrivateDirectory(directory); err != nil {
		return fmt.Errorf("create local draft directory: %w", err)
	}
	envelope := storedDraftEnvelope{SchemaVersion: localStoreSchemaVersion, SavedAt: s.now().UTC(), Draft: cloneDraft(draft)}
	return writeJSONAtomic(filepath.Join(directory, candidateID+".json"), envelope)
}

func (s *localStore) status() (model.LocalStorageStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return model.LocalStorageStatus{}, err
	}
	draftCount := 0
	for _, byCandidate := range state.Drafts {
		draftCount += len(byCandidate)
	}
	return model.LocalStorageStatus{
		Mode: "local-summary-only", RetentionDays: s.retentionDays,
		MatchSummaryCount: len(state.Matches), DraftCount: draftCount,
	}, nil
}

func (s *localStore) setRetention(days int) (model.LocalStorageStatus, error) {
	if !validRetention(days) {
		return model.LocalStorageStatus{}, fmt.Errorf("retention days must be %d..%d", minRetentionDays, maxRetentionDays)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.retentionDays
	s.retentionDays = days
	if err := s.writeSettings(); err != nil {
		s.retentionDays = previous
		return model.LocalStorageStatus{}, err
	}
	if err := s.cleanupExpiredLocked(); err != nil {
		return model.LocalStorageStatus{}, err
	}
	state, err := s.loadLocked()
	if err != nil {
		return model.LocalStorageStatus{}, err
	}
	draftCount := 0
	for _, byCandidate := range state.Drafts {
		draftCount += len(byCandidate)
	}
	return model.LocalStorageStatus{Mode: "local-summary-only", RetentionDays: days, MatchSummaryCount: len(state.Matches), DraftCount: draftCount}, nil
}

func (s *localStore) deleteMatch(matchID string) (model.DeleteLocalDataResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !safeStoreID(matchID) {
		return model.DeleteLocalDataResponse{}, errors.New("invalid local match identity")
	}
	deleted := model.DeleteLocalDataResponse{}
	path := filepath.Join(s.matchesDir, matchID+".json")
	if err := os.Remove(path); err == nil {
		deleted.DeletedMatches = 1
	} else if !errors.Is(err, os.ErrNotExist) {
		return model.DeleteLocalDataResponse{}, fmt.Errorf("delete local match summary: %w", err)
	}
	draftCount, err := s.removeDraftsLocked(matchID)
	if err != nil {
		return model.DeleteLocalDataResponse{}, err
	}
	deleted.DeletedDrafts = draftCount
	return deleted, nil
}

func (s *localStore) purge() (model.DeleteLocalDataResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := model.DeleteLocalDataResponse{}
	entries, err := os.ReadDir(s.matchesDir)
	if err != nil {
		return deleted, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		matchID := strings.TrimSuffix(entry.Name(), ".json")
		if !safeStoreID(matchID) {
			continue
		}
		if err := os.Remove(filepath.Join(s.matchesDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return deleted, err
		}
		deleted.DeletedMatches++
		count, err := s.removeDraftsLocked(matchID)
		if err != nil {
			return deleted, err
		}
		deleted.DeletedDrafts += count
	}
	draftDirectories, err := os.ReadDir(s.draftsDir)
	if err != nil {
		return deleted, err
	}
	for _, entry := range draftDirectories {
		if entry.Type()&os.ModeSymlink != 0 {
			return deleted, fmt.Errorf("refusing to follow local draft symlink %q", entry.Name())
		}
		if !entry.IsDir() || !safeStoreID(entry.Name()) {
			continue
		}
		count, err := s.removeDraftsLocked(entry.Name())
		if err != nil {
			return deleted, err
		}
		deleted.DeletedDrafts += count
	}
	return deleted, nil
}

func (s *localStore) cleanupExpiredLocked() error {
	cutoff := s.now().UTC().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	entries, err := os.ReadDir(s.matchesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		matchID := strings.TrimSuffix(entry.Name(), ".json")
		if !safeStoreID(matchID) {
			return fmt.Errorf("unsafe local match filename %q", entry.Name())
		}
		path := filepath.Join(s.matchesDir, entry.Name())
		var envelope storedMatchEnvelope
		if err := decodeStrictFile(path, &envelope); err != nil {
			return fmt.Errorf("read local match summary %q: %w", entry.Name(), err)
		}
		if envelope.SavedAt.Before(cutoff) {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if _, err := s.removeDraftsLocked(matchID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *localStore) removeDraftsLocked(matchID string) (int, error) {
	directory := filepath.Join(s.draftsDir, matchID)
	if err := validateExistingDirectory(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			return count, fmt.Errorf("unexpected directory in local draft store: %s", entry.Name())
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return count, err
		}
		if filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	if err := os.Remove(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return count, err
	}
	return count, nil
}

func (s *localStore) writeSettings() error {
	return writeJSONAtomic(s.settingsPath, localStoreSettings{SchemaVersion: localStoreSchemaVersion, RetentionDays: s.retentionDays})
}

func validateStoredMatch(envelope storedMatchEnvelope) error {
	if envelope.SchemaVersion != localStoreSchemaVersion || envelope.SavedAt.IsZero() || !safeStoreID(envelope.Match.ID) {
		return errors.New("invalid summary envelope")
	}
	if envelope.Match.Status != "finalized" || !envelope.Match.SavedLocally || envelope.Match.TimelineAvailable {
		return errors.New("stored match must be a finalized summary without a timeline")
	}
	return nil
}

func validRetention(days int) bool { return days >= minRetentionDays && days <= maxRetentionDays }

func safeStoreID(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func decodeStrictFile(path string, destination any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("local data file must be a regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("file must contain exactly one JSON value")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("local data directory must be a non-symlink directory")
	}
	return os.Chmod(path, 0o700)
}

func validateExistingDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("local data path must be a non-symlink directory")
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".pending-*.json")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return nil
}
