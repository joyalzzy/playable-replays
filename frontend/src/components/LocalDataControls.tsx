import { useState } from "react";
import type { LocalStorageStatus } from "../types";

type LocalDataControlsProps = {
  status: LocalStorageStatus | null;
  selectedMatchId: string;
  busy: boolean;
  onRetentionChange: (days: number) => Promise<void>;
  onDeleteSelected: () => Promise<void>;
  onDeleteAll: () => Promise<void>;
};

type Confirmation = "selected" | "all" | null;

const retentionOptions = [1, 7, 30, 90, 365];

export function LocalDataControls({
  status,
  selectedMatchId,
  busy,
  onRetentionChange,
  onDeleteSelected,
  onDeleteAll
}: LocalDataControlsProps) {
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const persistent = status?.mode === "local-summary-only";

  async function confirm(action: Exclude<Confirmation, null>) {
    if (action === "selected") await onDeleteSelected();
    else await onDeleteAll();
    setConfirmation(null);
  }

  return (
    <section className="local-data-controls" aria-labelledby="local-data-title">
      <div className="local-data-controls__heading">
        <div>
          <p className="eyebrow">LOCAL DATA CONTROL</p>
          <h3 id="local-data-title">Summary-only storage</h3>
          <p>Final match summaries and analyst drafts stay on this device. Collector tokens, source unit IDs, and raw telemetry frames are never saved.</p>
        </div>
        <strong className={`local-data-mode ${persistent ? "is-persistent" : ""}`}>
          {persistent ? "Local persistence on" : "Memory only"}
        </strong>
      </div>

      <div className="local-data-controls__grid">
        <label>
          Automatic deletion
          <select
            aria-label="Local data retention"
            disabled={!persistent || busy}
            value={status?.retentionDays || 7}
            onChange={(event) => void onRetentionChange(Number(event.target.value))}
          >
            {retentionOptions.map((days) => <option key={days} value={days}>{days === 1 ? "After 1 day" : `After ${days} days`}</option>)}
          </select>
        </label>
        <div><span>Saved match summaries</span><strong>{status?.matchSummaryCount ?? 0}</strong></div>
        <div><span>Saved drafts</span><strong>{status?.draftCount ?? 0}</strong></div>
      </div>

      <div className="local-data-controls__actions">
        <button type="button" disabled={!selectedMatchId || busy} onClick={() => setConfirmation("selected")}>Delete selected replay data</button>
        <button className="is-danger" type="button" disabled={busy || (!selectedMatchId && (status?.matchSummaryCount ?? 0) === 0)} onClick={() => setConfirmation("all")}>Delete all local telemetry</button>
      </div>

      {confirmation && (
        <div className="local-data-confirmation" role="alert">
          <p>{confirmation === "selected" ? `Delete ${selectedMatchId} and its saved drafts?` : "Delete every local telemetry summary and draft?"} This cannot be undone.</p>
          <div>
            <button className="is-danger" type="button" disabled={busy} onClick={() => void confirm(confirmation)}>{busy ? "Deleting…" : "Confirm deletion"}</button>
            <button type="button" disabled={busy} onClick={() => setConfirmation(null)}>Cancel</button>
          </div>
        </div>
      )}
    </section>
  );
}
