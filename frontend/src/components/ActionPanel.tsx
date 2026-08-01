import type { ActionType, Point } from "../types";

const labels: Record<ActionType, { title: string; detail: string }> = {
  move: { title: "Move", detail: "Reposition toward a chosen point" },
  hold: { title: "Hold", detail: "Recover slightly and keep formation" },
  contest: { title: "Contest", detail: "Commit to the nearest visible threat" },
  retreat: { title: "Retreat", detail: "Disengage toward the safe edge" },
  dodge: { title: "Dodge", detail: "Evade an incoming skillshot with a defensive sidestep" },
  outplay: { title: "Outplay", detail: "Attempt a high-risk mechanical counterplay" }
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
      <button
        className="commit"
        type="button"
        onClick={onCommit}
        disabled={disabled || (needsTarget && !target)}
      >
        {needsTarget && !target ? "Choose a point on the map" : "Commit decision"}
      </button>
    </section>
  );
}
