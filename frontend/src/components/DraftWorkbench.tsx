import { useEffect, useMemo, useState } from "react";
import {
  exportTelemetryReviewPack,
  previewTelemetryDraft,
  updateTelemetryDraft,
  validateTelemetryDraft
} from "../api";
import type {
  DraftField,
  DraftPreview,
  MechanicBriefing,
  TelemetryDraftResult,
  TelemetryScenario,
  TerrainFeature,
  Unit
} from "../types";

type DraftWorkbenchProps = {
  matchId: string;
  draft: TelemetryDraftResult;
  onChange: (draft: TelemetryDraftResult) => void;
  onPreview: (preview: DraftPreview) => void;
};

type EditableJSON = {
  units: string;
  terrain: string;
  rules: string;
  mechanics: string;
  alternatives: string;
  acceptanceTests: string;
};

type LocalErrors = Partial<Record<DraftField, string>>;

export function DraftWorkbench({ matchId, draft, onChange, onPreview }: DraftWorkbenchProps) {
  const [scenario, setScenario] = useState<TelemetryScenario>(() => draft.bundle.drafts[0].scenario);
  const [json, setJSON] = useState<EditableJSON>(() => editableJSON(draft.bundle.drafts[0].scenario));
  const [tradeoffs, setTradeoffs] = useState(() => draft.bundle.drafts[0].scenario.authoring.intendedTradeoffs.join("\n"));
  const [localErrors, setLocalErrors] = useState<LocalErrors>({});
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState<"save" | "validate" | "preview" | "export" | "">("");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    const next = draft.bundle.drafts[0].scenario;
    setScenario(next);
    setJSON(editableJSON(next));
    setTradeoffs(next.authoring.intendedTradeoffs.join("\n"));
    setLocalErrors({});
    setDirty(false);
  }, [draft.candidateId]);

  const issues = useMemo(() => {
    const grouped = new Map<DraftField, string[]>();
    for (const issue of draft.fieldIssues) {
      grouped.set(issue.field, [...(grouped.get(issue.field) ?? []), issue.message]);
    }
    return grouped;
  }, [draft.fieldIssues]);

  function setBasic<K extends keyof TelemetryScenario>(key: K, value: TelemetryScenario[K]) {
    setScenario((current) => ({ ...current, [key]: value }));
    setDirty(true);
  }

  function setAuthoring<K extends keyof TelemetryScenario["authoring"]>(
    key: K,
    value: TelemetryScenario["authoring"][K]
  ) {
    setScenario((current) => ({
      ...current,
      authoring: { ...current.authoring, [key]: value }
    }));
    setDirty(true);
  }

  function setJSONField(key: keyof EditableJSON, value: string) {
    setJSON((current) => ({ ...current, [key]: value }));
    setDirty(true);
  }

  function parseScenario(): TelemetryScenario | null {
    const errors: LocalErrors = {};
    const units = parseJSON<Unit[]>(json.units, "units", errors);
    const terrain = parseJSON<TerrainFeature[]>(json.terrain, "terrain", errors);
    const rules = parseJSON<Omit<TelemetryScenario["rules"], "terrain">>(json.rules, "rules", errors);
    const mechanics = parseJSON<MechanicBriefing | null>(json.mechanics, "terrain", errors);
    const alternatives = parseJSON<TelemetryScenario["authoring"]["plausibleAlternatives"]>(json.alternatives, "alternatives", errors);
    const acceptanceTests = parseJSON<TelemetryScenario["authoring"]["acceptanceTests"]>(json.acceptanceTests, "acceptanceTests", errors);
    setLocalErrors(errors);
    if (Object.keys(errors).length > 0 || !units || !terrain || !rules || alternatives === null || acceptanceTests === null) {
      return null;
    }
    return {
      ...scenario,
      mechanicBriefing: mechanics ?? undefined,
      units,
      rules: { ...rules, terrain },
      authoring: {
        ...scenario.authoring,
        intendedTradeoffs: tradeoffs.split("\n").map((value) => value.trim()).filter(Boolean),
        plausibleAlternatives: alternatives,
        acceptanceTests
      }
    };
  }

  async function save(): Promise<TelemetryDraftResult | null> {
    const next = parseScenario();
    if (!next) {
      setMessage("Fix the highlighted JSON before saving.");
      return null;
    }
    const result = await updateTelemetryDraft(matchId, draft.candidateId, next);
    setScenario(result.bundle.drafts[0].scenario);
    setDirty(false);
    onChange(result);
    return result;
  }

  async function run(operation: "save" | "validate" | "preview" | "export") {
    setBusy(operation);
    setMessage("");
    try {
      const saved = await save();
      if (!saved) return;
      if (operation === "save") {
        setMessage("Draft saved locally. The authored scenario library was not changed.");
        return;
      }
      const validated = await validateTelemetryDraft(matchId, draft.candidateId);
      onChange(validated);
      if (operation === "validate") {
        setMessage(validated.canPreview
          ? "Validation passed, including deterministic win and loss tests."
          : `Validation found ${validated.completionIssues.length} item${validated.completionIssues.length === 1 ? "" : "s"} to finish.`);
        return;
      }
      if (!validated.canPreview) {
        setMessage("Preview and export stay locked until every field validates and both outcome tests pass.");
        return;
      }
      if (operation === "preview") {
        onPreview(await previewTelemetryDraft(matchId, draft.candidateId));
        return;
      }
      const pack = await exportTelemetryReviewPack(matchId, draft.candidateId);
      downloadJSON(`${validated.bundle.drafts[0].scenario.slug}-review-pack.json`, pack);
      setMessage("A separate review pack was downloaded. moments.json was not modified.");
    } catch (caught) {
      setMessage(caught instanceof Error ? caught.message : "The workbench operation failed.");
    } finally {
      setBusy("");
    }
  }

  const source = scenario.sourceDetection;
  return (
    <section className="draft-workbench" aria-label="Analyst scenario workbench">
      <header className="draft-workbench__header">
        <div>
          <p className="eyebrow">ANALYST AUTHORING WORKBENCH</p>
          <h2>Turn the detected highlight into a playable lesson</h2>
          <p>Work is held by the local service only. Detector evidence is read-only, and the authored scenario library is never changed automatically.</p>
        </div>
        <span className={`draft-workbench__status draft-workbench__status--${draft.status}`}>
          {draft.status === "ready" ? "Ready for review" : "Publication locked"}
        </span>
      </header>

      <section className="draft-evidence">
        <div><span>Detected window</span><strong>{source.startSecond}s–{source.endSecond}s</strong></div>
        <div><span>Detection score</span><strong>{Math.round(source.score * 100)}/100</strong></div>
        <div><span>Mapped category</span><strong>{scenario.authoring.category.replaceAll("-", " ")}</strong></div>
        <div><span>Evidence</span><strong>{source.reasonTags.join(" · ")}</strong></div>
      </section>
      <FieldMessages field="provenance" issues={issues} localErrors={localErrors} />

      <div className="draft-workbench__grid">
        <fieldset className="draft-editor draft-editor--basics">
          <legend>Lesson basics</legend>
          <label className={fieldClass("title", issues, localErrors)}>Title<input value={scenario.title} onChange={(event) => setBasic("title", event.target.value)} /><FieldMessages field="title" issues={issues} localErrors={localErrors} /></label>
          <label className={fieldClass("description", issues, localErrors)}>Description<textarea rows={4} value={scenario.description} onChange={(event) => setBasic("description", event.target.value)} /><FieldMessages field="description" issues={issues} localErrors={localErrors} /></label>
          <div className="draft-editor__row">
            <label className={fieldClass("map", issues, localErrors)}>Map<input value={scenario.map} onChange={(event) => setBasic("map", event.target.value)} /><FieldMessages field="map" issues={issues} localErrors={localErrors} /></label>
            <label className={fieldClass("difficulty", issues, localErrors)}>Difficulty<select value={scenario.authoring.skillLevel} onChange={(event) => setAuthoring("skillLevel", event.target.value as TelemetryScenario["authoring"]["skillLevel"])}><option value="">Select</option><option value="beginner">Beginner</option><option value="intermediate">Intermediate</option><option value="advanced">Advanced</option></select><FieldMessages field="difficulty" issues={issues} localErrors={localErrors} /></label>
          </div>
          <div className="draft-editor__row">
            <label>Maximum turns<input type="number" min="1" max="12" value={scenario.maxTurns || ""} onChange={(event) => setBasic("maxTurns", Number(event.target.value))} /></label>
            <label>Controlled unit ID<input value={scenario.controlledUnitId} onChange={(event) => setBasic("controlledUnitId", event.target.value)} /></label>
          </div>
        </fieldset>

        <JSONEditor title="Synthetic units" field="units" value={json.units} rows={16} issues={issues} localErrors={localErrors} onChange={(value) => setJSONField("units", value)} />
        <JSONEditor title="Terrain" field="terrain" value={json.terrain} rows={14} issues={issues} localErrors={localErrors} onChange={(value) => setJSONField("terrain", value)} />
        <JSONEditor title="Simulator rules" field="rules" value={json.rules} rows={18} issues={issues} localErrors={localErrors} onChange={(value) => setJSONField("rules", value)} />
        <JSONEditor title="One-time mechanic briefing" field="terrain" value={json.mechanics} rows={10} issues={issues} localErrors={localErrors} onChange={(value) => setJSONField("mechanics", value)} />

        <fieldset className="draft-editor draft-editor--wide">
          <legend>Analyst reasoning</legend>
          <label className={fieldClass("rationale", issues, localErrors)}>Rationale<textarea rows={6} value={scenario.authoring.analystRationale} onChange={(event) => setAuthoring("analystRationale", event.target.value)} /><FieldMessages field="rationale" issues={issues} localErrors={localErrors} /></label>
          <label className={fieldClass("tradeoffs", issues, localErrors)}>Intended tradeoffs <small>One complete tradeoff per line; at least two.</small><textarea rows={5} value={tradeoffs} onChange={(event) => { setTradeoffs(event.target.value); setDirty(true); }} /><FieldMessages field="tradeoffs" issues={issues} localErrors={localErrors} /></label>
        </fieldset>

        <JSONEditor title="Plausible alternatives" field="alternatives" value={json.alternatives} rows={14} issues={issues} localErrors={localErrors} onChange={(value) => setJSONField("alternatives", value)} />
        <JSONEditor title="Win and loss acceptance tests" field="acceptanceTests" value={json.acceptanceTests} rows={18} issues={issues} localErrors={localErrors} onChange={(value) => setJSONField("acceptanceTests", value)} />
      </div>

      <section className="draft-validation" aria-live="polite">
        <div>
          <p className="eyebrow">DETERMINISTIC VALIDATION</p>
          <h3>{draft.canPreview ? "Both outcome paths pass" : "Preview remains locked"}</h3>
          {draft.acceptanceResults.length > 0 ? (
            <ul>{draft.acceptanceResults.map((result) => <li className={result.passed ? "is-pass" : "is-fail"} key={result.testName}><strong>{result.passed ? "Pass" : "Fail"}: {result.testName}</strong><span>{result.detail}</span></li>)}</ul>
          ) : <p>Add valid win and loss tests to run the simulator checks.</p>}
        </div>
        {draft.completionIssues.length > 0 && <ol>{draft.completionIssues.map((issue) => <li key={issue}>{issue}</li>)}</ol>}
      </section>

      {message && <p className="draft-workbench__message" role="status">{message}</p>}
      <footer className="draft-workbench__actions">
        <button type="button" disabled={Boolean(busy)} onClick={() => void run("save")}>{busy === "save" ? "Saving…" : "Save locally"}</button>
        <button type="button" disabled={Boolean(busy)} onClick={() => void run("validate")}>{busy === "validate" ? "Validating…" : "Validate"}</button>
        <button className="is-primary" type="button" disabled={Boolean(busy) || dirty || !draft.canPreview} onClick={() => void run("preview")}>{busy === "preview" ? "Starting…" : "Preview locally"}</button>
        <button type="button" disabled={Boolean(busy) || dirty || !draft.canExport} onClick={() => void run("export")}>{busy === "export" ? "Preparing…" : "Export review pack"}</button>
      </footer>
    </section>
  );
}

type JSONEditorProps = {
  title: string;
  field: DraftField;
  value: string;
  rows: number;
  issues: Map<DraftField, string[]>;
  localErrors: LocalErrors;
  onChange: (value: string) => void;
};

function JSONEditor({ title, field, value, rows, issues, localErrors, onChange }: JSONEditorProps) {
  return (
    <fieldset className={`draft-editor draft-editor--json ${fieldClass(field, issues, localErrors)}`}>
      <legend>{title}</legend>
      <p>Edit the structured simulator data. Validation explains the first rule that still needs attention.</p>
      <textarea aria-label={title} spellCheck={false} rows={rows} value={value} onChange={(event) => onChange(event.target.value)} />
      <FieldMessages field={field} issues={issues} localErrors={localErrors} />
    </fieldset>
  );
}

function FieldMessages({ field, issues, localErrors }: { field: DraftField; issues: Map<DraftField, string[]>; localErrors: LocalErrors }) {
  const messages = issues.get(field) ?? [];
  return (
    <>
      {localErrors[field] && <small className="draft-field-error">{localErrors[field]}</small>}
      {messages.map((message) => <small className="draft-field-error" key={message}>{message}</small>)}
    </>
  );
}

function fieldClass(field: DraftField, issues: Map<DraftField, string[]>, localErrors: LocalErrors): string {
  return issues.has(field) || localErrors[field] ? "draft-field--invalid" : "";
}

function editableJSON(scenario: TelemetryScenario): EditableJSON {
  const { terrain, ...rules } = scenario.rules;
  return {
    units: pretty(scenario.units),
    terrain: pretty(terrain),
    rules: pretty(rules),
    mechanics: pretty(scenario.mechanicBriefing ?? null),
    alternatives: pretty(scenario.authoring.plausibleAlternatives),
    acceptanceTests: pretty(scenario.authoring.acceptanceTests)
  };
}

function parseJSON<T>(value: string, field: DraftField, errors: LocalErrors): T | null {
  try {
    return JSON.parse(value) as T;
  } catch (caught) {
    errors[field] = caught instanceof Error ? caught.message : "This is not valid JSON.";
    return null;
  }
}

function pretty(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function downloadJSON(filename: string, value: unknown) {
  const url = URL.createObjectURL(new Blob([`${JSON.stringify(value, null, 2)}\n`], { type: "application/json" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}
