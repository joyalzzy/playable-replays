import type { ActionType, Point, Unit } from "../types";

const labels: Record<ActionType, { title: string; detail: string }> = {
  move: { title: "Move", detail: "Reposition using movement speed and terrain" },
  hold: { title: "Hold", detail: "Brace for damage and recover cooldowns" },
  contest: { title: "Contest", detail: "Pressure a visible enemy in attack range; attack when ready" },
  retreat: { title: "Retreat", detail: "Disengage toward the authored safe zone" }
};

type Props = {
  legalActions: ActionType[];
  selected: ActionType;
  target?: Point;
  contestTargets?: Unit[];
  selectedTargetUnitId?: string;
  disabled: boolean;
  onSelect: (action: ActionType) => void;
  onTargetUnit?: (unitId: string) => void;
  onCommit: () => void;
};

export function ActionPanel({
  legalActions,
  selected,
  target,
  contestTargets = [],
  selectedTargetUnitId,
  disabled,
  onSelect,
  onTargetUnit,
  onCommit
}: Props) {
  const needsMoveTarget = selected === "move";
  const needsContestTarget = selected === "contest";
  const selectedContestTarget = contestTargets.find((unit) => unit.id === selectedTargetUnitId);
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
      {needsMoveTarget && target && (
        <div className="coordinate-selection" role="status">
          <span>Selected movement point</span>
          <strong>X {target.x} · Y {target.y}</strong>
        </div>
      )}
      {needsContestTarget && (
        <label className="unit-target-selection">
          <span>Enemy inside controlled attack range</span>
          <select
            value={selectedContestTarget?.id ?? ""}
            onChange={(event) => onTargetUnit?.(event.target.value)}
            disabled={disabled || contestTargets.length === 0}
          >
            <option value="">{contestTargets.length === 0 ? "No attack target available" : "Select an enemy"}</option>
            {contestTargets.map((unit) => (
              <option key={unit.id} value={unit.id}>
                {unit.role} · {unit.class} · {unit.hp}/{unit.maxHp} HP
              </option>
            ))}
          </select>
        </label>
      )}
      <button
        className="commit"
        type="button"
        onClick={onCommit}
        disabled={disabled || (needsMoveTarget && !target) || (needsContestTarget && !selectedContestTarget)}
      >
        {needsMoveTarget
          ? target
            ? `Commit move to X ${target.x}, Y ${target.y}`
            : "Choose a point on the map"
          : needsContestTarget
            ? selectedContestTarget
              ? `Attack ${selectedContestTarget.role}`
              : "Select an enemy in attack range"
          : "Commit decision"}
      </button>
    </section>
  );
}
