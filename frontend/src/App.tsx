import { useEffect, useState } from "react";
import { createSession, listMoments, resetSession, takeTurn } from "./api";
import { ActionPanel } from "./components/ActionPanel";
import { OutcomeDebrief } from "./components/OutcomeDebrief";
import { TacticalBoard } from "./components/TacticalBoard";
import { Timeline } from "./components/Timeline";
import { actionLabel, advantageLabel } from "./format";
import { scenarioOptionLabel, sortMomentsByDifficulty } from "./scenarioOrder";
import type { ActionType, MomentSummary, Point, Session } from "./types";

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
  const [moments, setMoments] = useState<MomentSummary[]>([]);
  const [moment, setMoment] = useState<MomentSummary>();
  const [session, setSession] = useState<Session>();
  const [action, setAction] = useState<ActionType>("move");
  const [target, setTarget] = useState<Point>();
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState<string>();

  useEffect(() => {
    listMoments()
      .then(async (items) => {
        const orderedMoments = sortMomentsByDifficulty(items);
        setMoments(orderedMoments);
        const chosen = momentFromLocation(orderedMoments);
        if (!chosen) throw new Error("No replay moments are available.");
        setMoment(chosen);
        setSession(await createSession(chosen.id));
      })
      .catch((reason: unknown) =>
        setError(reason instanceof Error ? reason.message : "Could not load the replay.")
      )
      .finally(() => setBusy(false));
  }, []);

  async function chooseMoment(next: MomentSummary) {
    setBusy(true);
    setError(undefined);
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

  async function commit() {
    if (!session) return;
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
      setError(undefined);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The scenario could not be reset.");
    } finally {
      setBusy(false);
    }
  }

  if (!session || !moment) {
    return (
      <main className="shell shell--center">
        <div className="brand">PLAYABLE / REPLAYS</div>
        <p>{error ?? "Loading pivotal moment…"}</p>
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
            {moments.map((item) => (
              <option key={item.id} value={item.id}>{scenarioOptionLabel(item)}</option>
            ))}
          </select>
        </label>
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
            target={target}
            targeting={action === "move" && session.status === "active"}
            onTarget={setTarget}
          />
          <ActionPanel
            legalActions={session.legalActions}
            selected={action}
            target={target}
            disabled={busy || session.status !== "active"}
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
