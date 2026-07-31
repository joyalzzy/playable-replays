import { useEffect, useMemo, useState } from "react";
import { createSession, listMoments, resetSession, takeTurn } from "./api";
import { ActionPanel } from "./components/ActionPanel";
import { TacticalBoard } from "./components/TacticalBoard";
import { Timeline } from "./components/Timeline";
import type { ActionType, MomentSummary, Point, Session } from "./types";

function momentFromLocation(moments: MomentSummary[]) {
  const slug = new URLSearchParams(window.location.search).get("moment");
  return moments.find((moment) => moment.slug === slug) ?? moments[0];
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
        setMoments(items);
        const chosen = momentFromLocation(items);
        if (!chosen) throw new Error("No replay moments are available.");
        setMoment(chosen);
        setSession(await createSession(chosen.id));
      })
      .catch((reason: unknown) =>
        setError(reason instanceof Error ? reason.message : "Could not load the replay.")
      )
      .finally(() => setBusy(false));
  }, []);

  const controlledUnitId = useMemo(
    () => session?.units.find((unit) => unit.team === "blue" && unit.role === "carry")?.id ?? "",
    [session]
  );

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
      setTarget(undefined);
      setError(undefined);
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
              <option key={item.id} value={item.id}>{item.title}</option>
            ))}
          </select>
        </label>
      </header>

      <section className="brief">
        <div>
          <p className="eyebrow">PIVOTAL MOMENT · {moment.map}</p>
          <p>{moment.description}</p>
          <div className="tags">
            {moment.reasonTags.map((tag) => <span key={tag}>{tag.replaceAll("-", " ")}</span>)}
          </div>
        </div>
        <div className="meter">
          <span>Highlight confidence</span>
          <strong>{Math.round(moment.highlightScore * 100)}%</strong>
        </div>
      </section>

      <section className="stats" aria-label="Replay status">
        <div><span>Turn</span><strong>{session.turn}/{session.maxTurns}</strong></div>
        <div><span>Estimated win</span><strong>{Math.round(session.winProbability * 100)}%</strong></div>
        <div><span>Score</span><strong className={session.score < 0 ? "negative" : ""}>{session.score}</strong></div>
        <div><span>Status</span><strong>{session.status}</strong></div>
      </section>

      {error && <div className="error" role="alert">{error}</div>}
      {session.status !== "active" && (
        <div className={`result result--${session.status}`}>
          <strong>{session.status === "won" ? "Scenario secured" : "Scenario lost"}</strong>
          <span>This is a deterministic counterfactual estimate, not a factual match outcome.</span>
          <button type="button" onClick={() => void reset()} disabled={busy}>Replay moment</button>
        </div>
      )}

      <div className="workspace">
        <div>
          <TacticalBoard
            units={session.units}
            controlledUnitId={controlledUnitId}
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
            <p className="eyebrow">REFERENCE POLICY</p>
            <strong>{session.referenceAction.type}</strong>
            <span>Shown for learning; matching it is not required.</span>
          </section>
          <Timeline entries={session.log} />
          <button className="secondary" type="button" onClick={() => void reset()} disabled={busy}>
            Reset scenario
          </button>
        </aside>
      </div>

      <footer>
        Synthetic data · deterministic simulator · no proprietary game integration
      </footer>
    </main>
  );
}

