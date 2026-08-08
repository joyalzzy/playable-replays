import { useEffect, useState } from "react";
import { actionLabel, advantageLabel } from "../format";
import type { Session } from "../types";

type Props = {
  session: Session;
  busy: boolean;
  onReplay: () => void;
};

function featuredPlayerName(session: Session) {
  const featuredPlayer = session.units.find(
    (unit) => unit.team === "blue" && unit.policy === "controlled"
  );
  const roleSeparator = featuredPlayer?.role.indexOf("·") ?? -1;
  if (!featuredPlayer || roleSeparator < 1) return undefined;

  return featuredPlayer.role.slice(0, roleSeparator).trim() || undefined;
}

function possessive(name: string) {
  return name.toLocaleLowerCase().endsWith("s") ? `${name}’` : `${name}’s`;
}

export function OutcomeDebrief({ session, busy, onReplay }: Props) {
  const [showBestCase, setShowBestCase] = useState(false);
  const [selectedStep, setSelectedStep] = useState(0);

  useEffect(() => {
    setShowBestCase(false);
    setSelectedStep(0);
  }, [session.id, session.status]);

  if (session.status === "active") return null;

  const bestCase = session.bestCase;
  const activeStep = bestCase?.steps[Math.min(selectedStep, Math.max(0, bestCase.steps.length - 1))];
  const playerName = featuredPlayerName(session);
  const playerPossessive = playerName ? possessive(playerName) : undefined;

  return (
    <section className={`debrief debrief--${session.status}`} aria-label="Scenario debrief">
      <div className="debrief__header">
        <div>
          <p className="eyebrow">{playerName ? `${playerName.toLocaleUpperCase()} SCENARIO REVIEW` : "COUNTERFACTUAL DEBRIEF"}</p>
          <h2>{playerName ? `What would ${playerName} do here?` : session.status === "won" ? "Scenario secured" : "Scenario lost"}</h2>
          <p><strong>{session.status === "won" ? "Scenario secured." : "Scenario lost."}</strong> {session.outcomeReason}</p>
          {playerName && (
            <p className="debrief__context">
              This is an authored coaching counterfactual based on {playerPossessive} match context—not a claim about {playerPossessive} actual intent or an optimal real-world decision.
            </p>
          )}
        </div>
        <button type="button" onClick={onReplay} disabled={busy}>
          {playerPossessive ? `Replay ${playerPossessive} moment` : "Replay moment"}
        </button>
      </div>

      <ul className="debrief__facts">
        {(session.debrief ?? []).map((item) => <li key={item}>{item}</li>)}
      </ul>

      {bestCase && bestCase.steps.length > 0 && (
        <section className="best-case" aria-label="Calculated best allied line">
          <div className="best-case__intro">
            <div>
              <p className="eyebrow">CALCULATED BEST ALLIED LINE</p>
              <h3>{playerName ? `Inspect the strongest modeled line for ${playerName}` : "Inspect the strongest modeled sequence"}</h3>
              <p>
                Best reachable result: <strong>{bestCase.status}</strong> in {bestCase.turns} turns · {advantageLabel(bestCase.advantage)}.
              </p>
            </div>
            <button
              className="best-case__toggle"
              type="button"
              aria-expanded={showBestCase}
              onClick={() => {
                setShowBestCase((value) => !value);
                setSelectedStep(0);
              }}
            >
              {showBestCase ? "Hide best-case turns" : "Explore calculated best case"}
            </button>
          </div>

          {showBestCase && activeStep && (
            <div className="best-case__explorer">
              <p className="best-case__method">{bestCase.method} This is a simulation, not a guaranteed match outcome.</p>
              <div className="best-case__turns" aria-label="Best-case turn selector">
                {bestCase.steps.map((step, index) => (
                  <button
                    key={step.turn}
                    type="button"
                    className={index === selectedStep ? "best-case__turn best-case__turn--selected" : "best-case__turn"}
                    aria-label={`Turn ${step.turn}: ${actionLabel(step.action)}`}
                    aria-pressed={index === selectedStep}
                    onClick={() => setSelectedStep(index)}
                  >
                    <span>Turn {step.turn}</span>
                    <strong>{actionLabel(step.action)}</strong>
                  </button>
                ))}
              </div>

              <article className="best-case__detail">
                <div className="best-case__detail-heading">
                  <div>
                    <span>Best command · turn {activeStep.turn}</span>
                    <h4>{actionLabel(activeStep.action)}</h4>
                  </div>
                  <div className="best-case__swing">
                    <span>{advantageLabel(activeStep.advantageBefore)}</span>
                    <b aria-hidden="true">→</b>
                    <strong>{advantageLabel(activeStep.advantageAfter)}</strong>
                  </div>
                </div>
                <p className="best-case__reason">{activeStep.reason}</p>

                {activeStep.keyEvents.length > 0 && (
                  <div className="best-case__events">
                    <h5>What this turn caused</h5>
                    <ul>{activeStep.keyEvents.map((event) => <li key={event}>{event}</li>)}</ul>
                  </div>
                )}

                <div className="best-case__alternatives">
                  <h5>Strongest continuation after each available command</h5>
                  <div>
                    {activeStep.alternatives.map((alternative) => {
                      const chosen = alternative.action.type === activeStep.action.type;
                      return (
                        <article className={chosen ? "best-case-option best-case-option--chosen" : "best-case-option"} key={alternative.action.type}>
                          <span>{actionLabel(alternative.action)}{chosen ? " · selected" : ""}</span>
                          <strong>{alternative.status} · {advantageLabel(alternative.advantage)}</strong>
                          <small>{alternative.turns} turns to resolution</small>
                        </article>
                      );
                    })}
                  </div>
                </div>
              </article>
            </div>
          )}
        </section>
      )}

      <div className="comparison-heading">
        <div>
          <h3>{playerPossessive ? `Compare your opening in ${playerPossessive} scenario` : "First-decision comparison"}</h3>
          <p>Each card replays one legal opening through the same authored policy and scenario seed.</p>
        </div>
        <span>Reference rollouts · not historical outcomes</span>
      </div>

      <div className="comparison-grid">
        {(session.referenceOutcomes ?? []).map((outcome) => (
          <article className={`comparison-card comparison-card--${outcome.status}`} key={outcome.firstAction.type}>
            <div className="comparison-card__title">
              <strong>{actionLabel(outcome.firstAction)}</strong>
              <span>{outcome.status}</span>
            </div>
            <p>{outcome.outcomeReason}</p>
            <dl>
              <div><dt>Ending state</dt><dd>{advantageLabel(outcome.advantage)}</dd></div>
              <div><dt>Resolved in</dt><dd>{outcome.turns} turns</dd></div>
            </dl>
            {outcome.keyEvents.length > 0 && (
              <ul>
                {outcome.keyEvents.map((event) => <li key={event}>{event}</li>)}
              </ul>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}
