import { useId, type ReactNode } from "react";
import { replayRoleSymbols } from "../replaySymbols";

type BeginnerGuideProps = {
  onBack: () => void;
  onStartTutorial: () => void;
};

const commands = [
  { name: "Move", cue: "Reposition", description: "Choose a point on the map and travel toward it." },
  { name: "Hold", cue: "Defend", description: "Stay put, brace for damage, and recover your footing." },
  { name: "Contest", cue: "Pressure", description: "Approach the nearest visible threat and fight in range." },
  { name: "Retreat", cue: "Escape", description: "Disengage faster toward the scenario's marked safe area." }
] as const;

type GuideIconTooltipProps = {
  children: ReactNode;
  description: string;
  label: string;
  variant?: "role" | "signal";
};

function GuideIconTooltip({ children, description, label, variant = "role" }: GuideIconTooltipProps) {
  const tooltipID = useId();

  return (
    <button
      className={`beginner-guide__icon-tip beginner-guide__icon-tip--${variant}`}
      type="button"
      aria-label={`About ${label}`}
      aria-describedby={tooltipID}
    >
      {children}
      <span className="beginner-guide__icon-tooltip" id={tooltipID} role="tooltip">
        <strong>{label}</strong>
        <span>{description}</span>
      </span>
    </button>
  );
}

export function BeginnerGuide({ onBack, onStartTutorial }: BeginnerGuideProps) {
  return (
    <main className="beginner-guide">
      <header className="beginner-guide__topbar">
        <div>
          <p className="eyebrow">PLAYABLE / REPLAYS</p>
          <span>Beginner field guide</span>
        </div>
        <button className="menu-return" type="button" onClick={onBack}>Back to main menu</button>
      </header>

      <section className="beginner-guide__hero">
        <div className="beginner-guide__hero-copy">
          <p className="eyebrow">START HERE</p>
          <h1>Read the field.<br />Make the play.</h1>
          <p>
            Playable Replays pauses a tactical moment and gives you control of one blue
            character. Read the goal, choose one response, and learn from the consequence.
          </p>
          <button className="beginner-guide__start" type="button" onClick={onStartTutorial}>
            Start your first tutorial <span aria-hidden="true">→</span>
          </button>
        </div>

        <div className="beginner-guide__field" aria-label="Example tactical field">
          <span className="beginner-guide__field-label">YOUR VIEW</span>
          <div className="beginner-guide__field-ring beginner-guide__field-ring--outer" />
          <div className="beginner-guide__field-ring beginner-guide__field-ring--inner" />
          <div className="beginner-guide__field-unit beginner-guide__field-unit--player">
            <span>AD</span><small>You</small>
          </div>
          <div className="beginner-guide__field-unit beginner-guide__field-unit--ally">
            <span>+</span><small>Ally</small>
          </div>
          <div className="beginner-guide__field-unit beginner-guide__field-unit--enemy">
            <span>AP</span><small>Enemy</small>
          </div>
          <div className="beginner-guide__field-objective"><span /></div>
          <p><b>Gold ring</b> marks the character you control</p>
        </div>
      </section>

      <section className="beginner-guide__dashboard" aria-label="Beginner replay essentials">
        <article className="beginner-guide__panel beginner-guide__panel--turn">
          <header><span>01</span><h2>Your first turn</h2></header>
          <ol>
            <li><b>Read</b><span>Find the win condition and visible danger.</span></li>
            <li><b>Plan</b><span>Consider range, terrain, and hidden enemies.</span></li>
            <li><b>Commit</b><span>Choose once; every unit then responds.</span></li>
            <li><b>Review</b><span>Trace what your decision caused.</span></li>
          </ol>
        </article>

        <article className="beginner-guide__panel beginner-guide__panel--commands">
          <header><span>02</span><h2>Four core choices</h2></header>
          <div className="beginner-guide__commands">
            {commands.map((command) => (
              <div key={command.name}>
                <span>{command.cue}</span>
                <strong>{command.name}</strong>
                <p>{command.description}</p>
              </div>
            ))}
          </div>
        </article>

        <article className="beginner-guide__panel beginner-guide__panel--roles">
          <header><span>03</span><h2>Know the roles</h2></header>
          <div className="beginner-guide__roles">
            {replayRoleSymbols.map(({ role, mark, label, description }) => (
              <div key={role}>
                <GuideIconTooltip label={label} description={description}>
                  <span className={`replay-symbol replay-symbol--unit replay-symbol--role-${role}`} aria-hidden="true">
                    <span>{mark}</span>
                  </span>
                </GuideIconTooltip>
                <strong>{label}</strong>
              </div>
            ))}
          </div>
        </article>

        <article className="beginner-guide__panel beginner-guide__panel--signals">
          <header><span>04</span><h2>Three signals to watch</h2></header>
          <ul>
            <li>
              <GuideIconTooltip
                label="Gold ring"
                description="Identifies the one blue unit controlled by your commands. Other allied units follow the scenario's authored response policy."
                variant="signal"
              >
                <i className="beginner-guide__signal beginner-guide__signal--gold" />
              </GuideIconTooltip>
              <div><b>Gold ring</b><span>Your controlled character</span></div>
            </li>
            <li>
              <GuideIconTooltip
                label="Fog warning"
                description="Means at least one enemy position is unknown. Flanks and long escape routes may be unsafe until your team gains vision."
                variant="signal"
              >
                <i className="beginner-guide__signal beginner-guide__signal--fog">?</i>
              </GuideIconTooltip>
              <div><b>Fog warning</b><span>An enemy is unaccounted for</span></div>
            </li>
            <li>
              <GuideIconTooltip
                label="Crosshair"
                description="Marks the exact X/Y destination selected for Move. Your character travels toward this point when the turn is committed."
                variant="signal"
              >
                <i className="beginner-guide__signal beginner-guide__signal--target" />
              </GuideIconTooltip>
              <div><b>Crosshair</b><span>Your selected move destination</span></div>
            </li>
          </ul>
        </article>
      </section>

      <footer className="beginner-guide__footer">
        The tutorial's <b>?</b> button contains the complete in-replay symbol reference.
      </footer>
    </main>
  );
}
