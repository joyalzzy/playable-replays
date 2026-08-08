import { scenarioOptionLabel } from "../scenarioOrder";
import type { MomentSummary } from "../types";

type ModeIconProps = {
  kind: "tutorial" | "guide";
};

function ModeIcon({ kind }: ModeIconProps) {
  if (kind === "tutorial") {
    return (
      <svg className="mode-icon" viewBox="0 0 120 120" aria-hidden="true">
        <path d="M23 30 58 19l39 13v59L61 102 23 89Z" />
        <path d="m23 30 38 13 36-11M61 43v59" />
        <path className="mode-icon__accent" d="m48 58 25 14-25 14Z" />
        <circle cx="84" cy="48" r="5" />
      </svg>
    );
  }

  return (
    <svg className="mode-icon" viewBox="0 0 120 120" aria-hidden="true">
      <path d="M20 29c14-5 27-2 40 8v62C47 89 34 86 20 91Zm80 0c-14-5-27-2-40 8v62c13-10 26-13 40-8Z" />
      <path d="M32 48c7-1 13 1 18 5M32 63c7-1 13 1 18 5M70 53c6-4 12-6 18-5M70 68c6-4 12-6 18-5" />
      <circle className="mode-icon__accent" cx="60" cy="22" r="8" />
    </svg>
  );
}

type MainMenuProps = {
  moments: MomentSummary[];
  onOpenTutorial: () => void;
  onOpenGuide: () => void;
  onOpenScenario: (moment: MomentSummary) => void;
};

export function MainMenu({
  moments,
  onOpenTutorial,
  onOpenGuide,
  onOpenScenario
}: MainMenuProps) {
  return (
    <main className="menu-shell">
      <header className="menu-hero">
        <p className="eyebrow">PLAYABLE REPLAYS</p>
        <span className="menu-hero__status">TACTICAL DECISION LAB</span>
        <h1>Learn the moment.<br />Play the decision.</h1>
        <p>
          Replay pivotal full-map decisions, command one player, dodge heavy
          projectiles, and learn from the modeled response.
        </p>
      </header>

      <section className="mode-grid" aria-label="Playable Replays modes">
        <button
          aria-label="Open Tutorial"
          className="mode-card mode-card--featured"
          type="button"
          onClick={onOpenTutorial}
        >
          <span className="mode-card__badge">MAIN GAME</span>
          <span className="mode-card__number">01</span>
          <span className="mode-card__icon"><ModeIcon kind="tutorial" /></span>
          <span className="mode-card__copy">
            <small>FIXED SCENARIOS</small>
            <strong>Tutorial</strong>
            <span>Play one of the curated replay lessons on the complete tactical map.</span>
          </span>
          <span className="mode-card__action">Open tutorials <b>→</b></span>
        </button>

        <button
          aria-label="Open Beginner Guide"
          className="mode-card mode-card--guide"
          type="button"
          onClick={onOpenGuide}
        >
          <span className="mode-card__number">02</span>
          <span className="mode-card__icon"><ModeIcon kind="guide" /></span>
          <span className="mode-card__copy">
            <small>NEW PLAYER REFERENCE</small>
            <strong>Beginner Guide</strong>
            <span>Learn the map, character icons, commands, objectives, and outcome labels.</span>
          </span>
          <span className="mode-card__action">Read the guide <b>→</b></span>
        </button>
      </section>

      <section className="tutorial-library" aria-labelledby="tutorial-library-title">
        <header>
          <div>
            <p className="eyebrow">TUTORIAL SECTION</p>
            <h2 id="tutorial-library-title">Fixed scenario library</h2>
          </div>
          <span>{moments.length} authored {moments.length === 1 ? "lesson" : "lessons"}</span>
        </header>
        <div className="tutorial-library__grid">
          {moments.map((moment) => (
            <button key={moment.id} type="button" onClick={() => onOpenScenario(moment)}>
              <span>{moment.skillLevel}</span>
              <strong>{scenarioOptionLabel(moment).replace(/^\([^)]+\)\s*/, "")}</strong>
              <small>{moment.category.replaceAll("-", " ")}</small>
            </button>
          ))}
        </div>
      </section>

      <footer className="menu-footer">
        Replay-derived scenarios · authoritative simulation · deterministic fallback
      </footer>
    </main>
  );
}
