import type { LogEntry } from "../types";

const actorLabels: Record<LogEntry["actor"], string> = {
  user: "You",
  ally: "Ally",
  enemy: "Enemy",
  system: "State"
};

export function Timeline({ entries }: { entries: LogEntry[] }) {
  return (
    <section className="timeline">
      <h2>Causal decision trace</h2>
      {entries.length === 0 ? (
        <p className="muted">Movement, combat, vision, and objective consequences will appear here.</p>
      ) : (
        <ol>
          {entries.map((entry, index) => (
            <li className={`event event--${entry.kind}`} key={`${entry.turn}-${entry.actor}-${index}`}>
              <span className="event__meta">
                <span className={`actor actor--${entry.actor}`}>{actorLabels[entry.actor]}</span>
                <span>T{entry.turn} · {entry.kind}</span>
              </span>
              <span>{entry.message}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
