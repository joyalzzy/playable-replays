import type { MechanicBriefing } from "../types";

type Props = {
  briefing: MechanicBriefing;
  understood: boolean;
  disabled: boolean;
  onUnderstand: (understood: boolean) => void;
};

export function MechanicsBriefing({
  briefing,
  understood,
  disabled,
  onUnderstand
}: Props) {
  return (
    <section
      className={understood ? "mechanics-briefing mechanics-briefing--ready" : "mechanics-briefing"}
      aria-labelledby="mechanics-briefing-title"
    >
      <div className="mechanics-briefing__header">
        <div>
          <p className="eyebrow">BEFORE YOU DECIDE</p>
          <h2 id="mechanics-briefing-title">Scenario mechanic briefing</h2>
          <p>
            This replay introduces map mechanics unique to this situation. Read what each one does and why it matters here.
          </p>
        </div>
        <span className="mechanics-briefing__status" role="status">
          {understood ? "Commands unlocked" : "Input locked"}
        </span>
      </div>

      <div className="mechanics-briefing__grid">
        {briefing.mechanics.map((mechanic) => (
          <article key={mechanic.elementId}>
            <h3>{mechanic.name}</h3>
            <dl>
              <div>
                <dt>What it does</dt>
                <dd>{mechanic.description}</dd>
              </div>
              <div>
                <dt>Role in this replay</dt>
                <dd>{mechanic.roleInScenario}</dd>
              </div>
            </dl>
          </article>
        ))}
      </div>

      <label className="mechanics-briefing__acknowledgement">
        <input
          type="checkbox"
          checked={understood}
          disabled={disabled}
          onChange={(event) => onUnderstand(event.target.checked)}
        />
        <span>
          <strong>I understand these scenario mechanics</strong>
          <small>Check this box to unlock the map and command choices below.</small>
        </span>
      </label>
    </section>
  );
}
