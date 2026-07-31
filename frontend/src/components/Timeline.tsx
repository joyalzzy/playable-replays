import type { LogEntry } from "../types";

export function Timeline({ entries }: { entries: LogEntry[] }) {
  return (
    <section className="timeline">
      <h2>Decision trace</h2>
      {entries.length === 0 ? (
        <p className="muted">Your choices and the policy response will appear here.</p>
      ) : (
        <ol>
          {entries.map((entry, index) => (
            <li key={`${entry.turn}-${entry.actor}-${index}`}>
              <span className={`actor actor--${entry.actor}`}>
                {entry.actor === "user" ? "You" : "Policy"}
              </span>
              <span>{entry.message}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

