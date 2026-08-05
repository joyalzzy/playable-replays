import type { ActionType, Point } from "../types";

const labels: Record<ActionType, { title: string; detail: string }> = {
  move: { title: "Move", detail: "Reposition using movement speed and terrain" },
  hold: { title: "Hold", detail: "Brace for damage and recover cooldowns" },
  contest: { title: "Contest", detail: "Close on the nearest visible threat and attack in range" },
  retreat: { title: "Retreat", detail: "Disengage toward the authored safe zone" }
};

type Props = {
  legalActions: ActionType[];
  selected: ActionType;
  target?: Point;
  disabled: boolean;
  onSelect: (action: ActionType) => void;
  onCommit: () => void;
};

export function ActionPanel({
  legalActions,
  selected,
  target,
  disabled,
  onSelect,
  onCommit
}: Props) {
  const needsTarget = selected === "move";
  return (
    <section className="action-panel" aria-label="Command panel">
      <div className="action-grid">
        {legalActions.map((action) => (
          <button
            key={action}
            type="button"
            className={selected === action ? "action action--selected" : "action"}
            onClick={() => onSelect(action)}
            disabled={disabled}
          >
            <strong>{labels[action].title}</strong>
            <span>{labels[action].detail}</span>
          </button>
        ))}
      </div>
      {needsTarget && target && (
        <div className="coordinate-selection" role="status">
          <span>Selected movement point</span>
          <strong>X {target.x} · Y {target.y}</strong>
        </div>
      )}
      <button
        className="commit"
        type="button"
        onClick={onCommit}
        disabled={disabled || (needsTarget && !target)}
      >
        {needsTarget
          ? target
            ? `Commit move to X ${target.x}, Y ${target.y}`
            : "Choose a point on the map"
          : "Commit decision"}
      </button>
    </section>
  );
}
