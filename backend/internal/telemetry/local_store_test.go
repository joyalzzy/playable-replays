package telemetry

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joyalzzy/playable-replays/backend/internal/drafts"
	"github.com/joyalzzy/playable-replays/backend/internal/model"
)

func TestPersistentServiceStoresOnlyAnonymousSummariesAndDrafts(t *testing.T) {
	directory := t.TempDir()
	service, err := NewPersistentService(directory, 7)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Start(model.CreateTelemetryMatchRequest{Source: "authorized", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	for second := 0; second <= 12; second++ {
		frame := reversalFrame(second)
		frame.Units[0].ID = "secret-player-123"
		frame.Units[1].ID = "opponent-account-999"
		batch := model.TelemetryFrameBatch{SchemaVersion: "1.0", Sequence: second, Frames: []model.LiveTelemetryFrame{frame}}
		if _, err := service.Ingest(created.Match.ID, created.CollectorToken, batch); err != nil {
			t.Fatalf("ingest second %d: %v", second, err)
		}
	}
	finalized, err := service.Finish(created.Match.ID, created.CollectorToken)
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.SavedLocally || !finalized.TimelineAvailable {
		t.Fatalf("live finalized match should report a saved summary and memory timeline: %+v", finalized)
	}
	if len(finalized.Candidates) != 1 {
		t.Fatalf("expected a final candidate: %+v", finalized)
	}
	generated, err := service.Draft(finalized.ID, finalized.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateDraft(finalized.ID, finalized.Candidates[0].ID, generated.Bundle.Drafts[0].Scenario); err != nil {
		t.Fatalf("replace persisted draft: %v", err)
	}

	contents := readStoreFiles(t, directory)
	for _, forbidden := range []string{created.CollectorToken, "secret-player-123", "opponent-account-999"} {
		if strings.Contains(contents, forbidden) {
			t.Fatalf("local summary store persisted forbidden telemetry %q:\n%s", forbidden, contents)
		}
	}
	if !strings.Contains(contents, `"oneVersusManyUnitIds"`) || strings.Contains(contents, `"collectorToken"`) {
		t.Fatalf("stored evidence was missing or a token field was present:\n%s", contents)
	}

	restarted, err := NewPersistentService(directory, 7)
	if err != nil {
		t.Fatal(err)
	}
	matches := restarted.List()
	if len(matches) != 1 || !matches[0].SavedLocally || matches[0].TimelineAvailable {
		t.Fatalf("restart did not restore a summary-only match: %+v", matches)
	}
	if _, err := restarted.Timeline(matches[0].ID); !errors.Is(err, ErrTimelineAbsent) {
		t.Fatalf("restored summary should not claim a persisted timeline, got %v", err)
	}
	if _, err := restarted.GetDraft(matches[0].ID, matches[0].Candidates[0].ID); err != nil {
		t.Fatalf("restart did not restore the analyst draft: %v", err)
	}
	deleted, err := restarted.Delete(matches[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedMatches != 1 || deleted.DeletedDrafts != 1 {
		t.Fatalf("unexpected deletion result: %+v", deleted)
	}
	status, err := restarted.StorageStatus()
	if err != nil || status.MatchSummaryCount != 0 || status.DraftCount != 0 {
		t.Fatalf("deleted data remains in the local store: %+v, %v", status, err)
	}
}

func TestLocalStoreRetentionDeletesExpiredSummaryAndDraft(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	store, err := openLocalStoreWithClock(directory, 30, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	match := model.TelemetryMatch{ID: "telemetry-match-1", Source: "synthetic", Status: "finalized", SavedLocally: true}
	if err := store.saveMatch(match); err != nil {
		t.Fatal(err)
	}
	draft := draftsForStoreTest()
	if err := store.saveDraft(match.ID, "candidate-0-12", draft); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	status, err := store.setRetention(7)
	if err != nil {
		t.Fatal(err)
	}
	if status.MatchSummaryCount != 0 || status.DraftCount != 0 {
		t.Fatalf("expired data survived retention cleanup: %+v", status)
	}
}

func TestLocalStoreRejectsTrailingJSON(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "matches"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "drafts"), 0o700); err != nil {
		t.Fatal(err)
	}
	settings := `{"schemaVersion":"1.0","retentionDays":7} trailing`
	if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openLocalStore(directory, 7); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("malformed trailing storage JSON should be rejected, got %v", err)
	}
}

func TestLocalStorePurgeRemovesOrphanDraft(t *testing.T) {
	store, err := openLocalStore(t.TempDir(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.saveDraft("telemetry-match-9", "candidate-0-12", draftsForStoreTest()); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.purge()
	if err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedMatches != 0 || deleted.DeletedDrafts != 1 {
		t.Fatalf("orphan draft was not purged: %+v", deleted)
	}
}

func draftsForStoreTest() drafts.Draft {
	return drafts.Draft{Status: drafts.DraftStatus, Scenario: model.Moment{ID: "draft-1", Slug: "draft-1"}}
}

func readStoreFiles(t *testing.T, root string) string {
	t.Helper()
	var combined strings.Builder
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		combined.Write(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return combined.String()
}
