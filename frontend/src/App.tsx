import { useEffect, useState } from "react";
import { createSession, listMoments, resetSession, takeTurn } from "./api";
import { ActionPanel } from "./components/ActionPanel";
import { BeginnerGuide } from "./components/BeginnerGuide";
import { MainMenu } from "./components/MainMenu";
import { MechanicsBriefing } from "./components/MechanicsBriefing";
import { OutcomeDebrief } from "./components/OutcomeDebrief";
import { ReplayHelp } from "./components/ReplayHelp";
import { TacticalBoard } from "./components/TacticalBoard";
import { TelemetryHub } from "./components/TelemetryHub";
import { Timeline } from "./components/Timeline";
import { actionLabel, advantageLabel } from "./format";
import { mapViewportForMoment } from "./mapViewport";
import { scenarioOptionLabel, sortMomentsByDifficulty } from "./scenarioOrder";
import type { ActionType, DraftPreview, MomentSummary, Point, Session } from "./types";

type AppView = "menu" | "tutorial" | "telemetry" | "guide";

function viewFromLocation(): AppView {
  const parameters = new URLSearchParams(window.location.search);
  if (parameters.has("moment") || parameters.get("view") === "tutorial") return "tutorial";
  if (parameters.get("view") === "telemetry") return "telemetry";
  if (parameters.get("view") === "guide") return "guide";
  return "menu";
}

function momentFromLocation(moments: MomentSummary[]) {
  const slug = new URLSearchParams(window.location.search).get("moment");
  return moments.find((moment) => moment.slug === slug) ?? moments[0];
}

function objectiveSummary(session: Session) {
  if (session.objective) {
    return `${session.objective.blueProgress}/${session.objective.requiredProgress} blue`;
  }
  return `${session.escapeProgress}/${session.escapeTurnsRequired} safe`;
}

export default function App() {
  const [view, setView] = useState<AppView>(viewFromLocation);
  const [moments, setMoments] = useState<MomentSummary[]>([]);
  const [moment, setMoment] = useState<MomentSummary>();
  const [session, setSession] = useState<Session>();
  const [action, setAction] = useState<ActionType>("move");
  const [target, setTarget] = useState<Point>();
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState<string>();
  const [mechanicsUnderstood, setMechanicsUnderstood] = useState(false);
  const mechanicsLocked = Boolean(session?.mechanicBriefing && !mechanicsUnderstood);

  useEffect(() => {
    listMoments()
      .then(async (items) => {
        const orderedMoments = sortMomentsByDifficulty(items);
        setMoments(orderedMoments);
        if (viewFromLocation() === "tutorial") {
          const chosen = momentFromLocation(orderedMoments);
          if (!chosen) throw new Error("No replay moments are available.");
          setMoment(chosen);
          setSession(await createSession(chosen.id));
        }
      })
      .catch((reason: unknown) =>
        setError(reason instanceof Error ? reason.message : "Could not load the replay.")
      )
      .finally(() => setBusy(false));
  }, []);

  async function chooseMoment(next: MomentSummary) {
    setView("tutorial");
    setBusy(true);
    setError(undefined);
    setMechanicsUnderstood(false);
    try {
      const nextSession = await createSession(next.id);
      setMoment(next);
      setSession(nextSession);
      setAction("move");
      setTarget(undefined);
      window.history.replaceState(null, "", `?moment=${encodeURIComponent(next.slug)}`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not start the replay.");
    } finally {
      setBusy(false);
    }
  }

  function openMenu() {
    setView("menu");
    window.history.replaceState(null, "", "?view=menu");
  }

  function openTelemetry() {
    setView("telemetry");
    window.history.replaceState(null, "", "?view=telemetry");
  }

  function openGuide() {
    setView("guide");
    window.history.replaceState(null, "", "?view=guide");
  }

  function openTutorial() {
    const chosen = moment ?? moments[0];
    if (chosen) void chooseMoment(chosen);
  }

  function openDraftPreview(preview: DraftPreview) {
    setView("tutorial");
    setMoment(preview.moment);
    setSession(preview.session);
    setAction("move");
    setTarget(undefined);
    setMechanicsUnderstood(false);
    setError(undefined);
    window.history.replaceState(null, "", "?view=tutorial&preview=local");
  }

  async function commit() {
    if (!session || mechanicsLocked) return;
    setBusy(true);
    setError(undefined);
    try {
      setSession(
        await takeTurn(session.id, {
          type: action,
          ...(action === "move" && target ? { target } : {})
        })
      );
      setTarget(undefined);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The turn could not be resolved.");
    } finally {
      setBusy(false);
    }
  }

  async function reset() {
    if (!session) return;
    setBusy(true);
    try {
      setSession(await resetSession(session.id));
      setAction("move");
      setTarget(undefined);
      setMechanicsUnderstood(false);
      setError(undefined);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The scenario could not be reset.");
    } finally {
      setBusy(false);
    }
  }

  if ((busy || error) && moments.length === 0) {
    return (
      <main className="shell shell--center">
        <div className="brand">PLAYABLE / REPLAYS</div>
        <p>{error ?? "Loading pivotal moment…"}</p>
      </main>
    );
  }

  if (view === "menu") {
    return (
      <MainMenu
        moments={moments}
        onOpenTutorial={openTutorial}
        onOpenTelemetry={openTelemetry}
        onOpenGuide={openGuide}
        onOpenScenario={(next) => void chooseMoment(next)}
      />
    );
  }

  if (view === "telemetry") {
    return <TelemetryHub onBack={openMenu} onOpenTutorial={openTutorial} onPreviewDraft={openDraftPreview} />;
  }

  if (view === "guide") {
    return <BeginnerGuide onBack={openMenu} onStartTutorial={openTutorial} />;
  }

  if (!session || !moment) {
    return (
      <main className="shell shell--center">
        <div className="brand">PLAYABLE / REPLAYS</div>
        <p>{error ?? "Preparing tutorial…"}</p>
      </main>
    );
  }

  return (
    <main className="shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">PLAYABLE / REPLAYS</p>
          <h1>{moment.title}</h1>
        </div>
        <div className="topbar__controls">
          <button className="menu-return" type="button" onClick={openMenu}>Main menu</button>
          <label>
            <span className="sr-only">Select replay moment</span>
            <select
              value={moment.id}
              onChange={(event) => {
                const next = moments.find((item) => item.id === event.target.value);
                if (next) void chooseMoment(next);
              }}
              disabled={busy}
            >
              {!moments.some((item) => item.id === moment.id) && (
                <option value={moment.id}>[Local preview] {moment.title}</option>
              )}
              {moments.map((item) => (
                <option key={item.id} value={item.id}>{scenarioOptionLabel(item)}</option>
              ))}
            </select>
          </label>
          <ReplayHelp />
        </div>
      </header>

      <section className="brief">
        <div>
          <p className="eyebrow">PIVOTAL MOMENT · {moment.map}</p>
          <p>{moment.description}</p>
          <div className="tags">
            <span>{moment.skillLevel}</span>
            <span>{moment.category.replaceAll("-", " ")}</span>
            {moment.reasonTags.map((tag) => <span key={tag}>{tag.replaceAll("-", " ")}</span>)}
          </div>
        </div>
        <div className="meter">
          <span>Highlight confidence</span>
          <strong>{Math.round(moment.highlightScore * 100)}%</strong>
        </div>
      </section>

      <section className="goal-card">
        <div>
          <p className="eyebrow">EXPLICIT WIN CONDITION</p>
          <strong>{session.scenarioGoal}</strong>
        </div>
        {session.visionLimited && (
          <span className="goal-card__warning">Limited vision · {session.unknownEnemyCount} unknown enemy contact{session.unknownEnemyCount === 1 ? "" : "s"}</span>
        )}
      </section>

      <section className="stats" aria-label="Replay status">
        <div><span>Turn</span><strong>{session.turn}/{session.maxTurns}</strong></div>
        <div><span>Scenario advantage</span><strong>{advantageLabel(session.advantage)}</strong></div>
        <div><span>{session.objective?.label ?? "Escape route"}</span><strong>{objectiveSummary(session)}</strong></div>
        <div><span>Known threats</span><strong>{session.visibleEnemyCount} visible · {session.unknownEnemyCount} unknown</strong></div>
      </section>

      {error && <div className="error" role="alert">{error}</div>}
      <OutcomeDebrief session={session} busy={busy} onReplay={() => void reset()} />

      <div className="workspace">
        <div>
          <TacticalBoard
            units={session.units}
            terrain={session.terrain}
            objective={session.objective}
            controlledUnitId={session.controlledUnitId}
            unknownEnemyCount={session.unknownEnemyCount}
            viewport={mapViewportForMoment(session.momentId)}
            target={target}
            targeting={action === "move" && session.status === "active" && !mechanicsLocked}
            onTarget={setTarget}
          />
          {session.mechanicBriefing && (
            <MechanicsBriefing
              briefing={session.mechanicBriefing}
              understood={mechanicsUnderstood}
              disabled={busy || session.status !== "active"}
              onUnderstand={(understood) => {
                setMechanicsUnderstood(understood);
                if (!understood) setTarget(undefined);
              }}
            />
          )}
          <ActionPanel
            legalActions={session.legalActions}
            selected={action}
            target={target}
            disabled={busy || session.status !== "active" || mechanicsLocked}
            onSelect={(next) => {
              setAction(next);
              if (next !== "move") setTarget(undefined);
            }}
            onCommit={() => void commit()}
          />
        </div>
        <aside>
          <section className="reference">
            <p className="eyebrow">POST-COMMIT REFERENCE</p>
            {session.lastReferenceAction ? (
              <>
                <strong>{actionLabel(session.lastReferenceAction)}</strong>
                <span>{session.referenceReason}</span>
              </>
            ) : (
              <>
                <strong className="reference__hidden">Hidden until you decide</strong>
                <span>Your first choice remains independent. The authored baseline appears after commitment.</span>
              </>
            )}
          </section>
          <Timeline entries={session.log} />
          <button className="secondary" type="button" onClick={() => void reset()} disabled={busy}>
            Reset scenario
          </button>
        </aside>
      </div>

      <footer>
        Synthetic data · deterministic authored rules · advantage is not a win probability
      </footer>
    </main>
  );
}
