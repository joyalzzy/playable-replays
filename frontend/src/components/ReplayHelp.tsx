import { useEffect, useRef, useState, type ReactNode } from "react";
import { replayRoleSymbols } from "../replaySymbols";

const helpPanelID = "replay-help-panel";

type LegendCardProps = {
  title: string;
  description: string;
  symbol: ReactNode;
};

function LegendCard({ title, description, symbol }: LegendCardProps) {
  return (
    <article className="replay-help__legend-card" aria-label={title}>
      {symbol}
      <div>
        <strong>{title}</strong>
        <p>{description}</p>
      </div>
    </article>
  );
}

type UnitSymbolProps = {
  role: string;
  mark: string;
  variant?: "ally" | "controlled" | "enemy" | "guarded" | "shield";
};

function UnitSymbol({ role, mark, variant = "ally" }: UnitSymbolProps) {
  return (
    <span
      className={`replay-symbol replay-symbol--unit replay-symbol--role-${role} replay-symbol--${variant}`}
      aria-hidden="true"
    >
      <span>{mark}</span>
      {variant === "shield" && <small>+4</small>}
    </span>
  );
}

export function ReplayHelp() {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return undefined;

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
        triggerRef.current?.focus();
      }
    }

    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [open]);

  return (
    <div className="replay-help">
      <button
        ref={triggerRef}
        className="replay-help__trigger"
        type="button"
        aria-label="How replay decisions work"
        aria-expanded={open}
        aria-controls={helpPanelID}
        onClick={() => setOpen(!open)}
      >
        ?
      </button>

      {open && (
        <section
          id={helpPanelID}
          className="replay-help__panel"
          role="dialog"
          aria-labelledby="replay-help-title"
        >
          <header className="replay-help__header">
            <div>
              <p className="eyebrow">BEGINNER GUIDE</p>
              <h2 id="replay-help-title">How the replay works</h2>
            </div>
            <button
              className="replay-help__close"
              type="button"
              aria-label="Close replay help"
              onClick={() => {
                setOpen(false);
                triggerRef.current?.focus();
              }}
            >
              Close
            </button>
          </header>

          <p className="replay-help__intro">
            You control the blue character with the gold ring. Each turn, choose one command
            for that character, then commit it so the simulator can resolve everyone else's response.
          </p>

          <ol className="replay-help__steps">
            <li><span>1</span><div><strong>Read the goal</strong><p>Check the win condition, visible threats, and unknown contacts.</p></div></li>
            <li><span>2</span><div><strong>Choose a command</strong><p>Think about safety, range, terrain, and what the opposing team may do next.</p></div></li>
            <li><span>3</span><div><strong>Commit the turn</strong><p>Non-player units receive complete tactical commands from the external model, with a deterministic fallback if it cannot respond.</p></div></li>
            <li><span>4</span><div><strong>Review the result</strong><p>Use the causal trace and post-commit reference to understand what your choice caused.</p></div></li>
          </ol>

          <h3>Character role icons</h3>
          <p className="replay-help__section-intro">
            The letters inside a character marker identify their usual MOBA role. The border
            color tells you which team they belong to.
          </p>
          <div className="replay-help__legend replay-help__legend--roles">
            {replayRoleSymbols.map(({ role, mark, label, description }) => (
              <LegendCard
                key={role}
                title={label}
                description={description}
                symbol={<UnitSymbol role={role} mark={mark} />}
              />
            ))}
          </div>

          <h3>Team and status indicators</h3>
          <div className="replay-help__legend">
            <LegendCard
              title="Your character"
              description="The blue marker with a gold outer ring is the character controlled by your choices."
              symbol={<UnitSymbol role="carry" mark="AD" variant="controlled" />}
            />
            <LegendCard
              title="Ally"
              description="A cyan border and dot identify a visible character on your team."
              symbol={<UnitSymbol role="support" mark="+" />}
            />
            <LegendCard
              title="Enemy"
              description="A red border and dot identify a visible opposing character."
              symbol={<UnitSymbol role="mage" mark="AP" variant="enemy" />}
            />
            <LegendCard
              title="Guarded"
              description="A cyan aura means the character is braced and takes less incoming damage this turn."
              symbol={<UnitSymbol role="tank" mark="T" variant="guarded" />}
            />
            <LegendCard
              title="Shield"
              description="A +number badge is temporary protection that absorbs damage before health."
              symbol={<UnitSymbol role="support" mark="+" variant="shield" />}
            />
            <LegendCard
              title="Health"
              description="The green bar shows remaining health. An empty bar means the character is eliminated."
              symbol={<span className="replay-symbol replay-symbol--health" aria-hidden="true"><span /></span>}
            />
            <LegendCard
              title="Limited vision"
              description="The amber warning means one or more enemies are hidden and may approach from fog."
              symbol={<span className="replay-symbol replay-symbol--vision" aria-hidden="true">?</span>}
            />
          </div>

          <h3>Map and terrain symbols</h3>
          <div className="replay-help__legend">
            <LegendCard
              title="Objective core"
              description="Stand and contest inside this ring to build team control before the opponent does."
              symbol={<span className="replay-symbol replay-symbol--objective" aria-hidden="true"><span /></span>}
            />
            <LegendCard
              title="Blue base"
              description="The dashed blue area marks the Blue base destination used when retreating."
              symbol={<span className="replay-symbol replay-symbol--safe" aria-hidden="true">BASE</span>}
            />
            <LegendCard
              title="Brush"
              description="Green brush can block long-range vision and hide an approaching character."
              symbol={<span className="replay-symbol replay-symbol--brush" aria-hidden="true" />}
            />
            <LegendCard
              title="Wall"
              description="Stone walls obstruct sight lines and slow routes that pass around them."
              symbol={<span className="replay-symbol replay-symbol--wall" aria-hidden="true" />}
            />
            <LegendCard
              title="River"
              description="The blue river strip is a fixed map route where central objective fights happen."
              symbol={<span className="replay-symbol replay-symbol--river" aria-hidden="true" />}
            />
            <LegendCard
              title="Move target"
              description="The gold crosshair marks the X/Y destination selected for your next Move command."
              symbol={<span className="replay-symbol replay-symbol--target" aria-hidden="true" />}
            />
            <LegendCard
              title="Team base"
              description="The large platform and diamond mark a team base: cyan for blue and red for the opponent."
              symbol={<span className="replay-symbol replay-symbol--base" aria-hidden="true"><span /></span>}
            />
            <LegendCard
              title="Base gate"
              description="The paired pylons mark the fixed junction where all three lanes connect to a team base."
              symbol={<span className="replay-symbol replay-symbol--gate" aria-hidden="true"><span /><span /></span>}
            />
            <LegendCard
              title="Lane turret"
              description="Each team has one turret in every lane. Cyan and red styling identify which side owns it."
              symbol={<span className="replay-symbol replay-symbol--turret" aria-hidden="true"><span /></span>}
            />
            <LegendCard
              title="Marksman projectile"
              description="A bright bolt is a pending marksman shot worth half your character's maximum health. Dodge before committing another command."
              symbol={<span className="replay-symbol replay-symbol--projectile" aria-hidden="true"><span /></span>}
            />
          </div>

          <h3>What each choice does</h3>
          <div className="replay-help__choices">
            <article>
              <strong>Move</strong>
              <p>Pick an X/Y point. Your character travels toward it up to their movement limit; terrain can slow the route.</p>
            </article>
            <article>
              <strong>Hold</strong>
              <p>Stay in place, gain 4 shield, and reduce incoming damage for this turn.</p>
            </article>
            <article>
              <strong>Contest</strong>
              <p>Focus the visible priority target; otherwise close on the nearest visible enemy and attack in range. With no visible target, advance on the objective or hold.</p>
            </article>
            <article>
              <strong>Retreat</strong>
              <p>Move 20% faster toward the Blue base and reduce incoming damage while disengaging.</p>
            </article>
          </div>

          <h3>Separate projectile response</h3>
          <article className="replay-help__dodge">
            <strong>Dodge · two charges</strong>
            <p>
              Dodge is not a tactical command. When a half-health marksman projectile is pending, its
              dedicated button evades the shot immediately without advancing the tactical turn.
              Each scenario provides two uses.
            </p>
          </article>

          <p className="replay-help__note">
            Outcomes are deterministic estimates for this authored scenario, not guaranteed results from a real match.
          </p>
        </section>
      )}
    </div>
  );
}
