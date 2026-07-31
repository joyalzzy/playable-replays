import type { MouseEvent } from "react";
import type { Point, Unit } from "../types";

type Props = {
  units: Unit[];
  controlledUnitId: string;
  target?: Point;
  targeting: boolean;
  onTarget: (point: Point) => void;
};

export function TacticalBoard({
  units,
  controlledUnitId,
  target,
  targeting,
  onTarget
}: Props) {
  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    if (!targeting) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    onTarget({
      x: Math.round(((event.clientX - bounds.left) / bounds.width) * 100),
      y: Math.round(((event.clientY - bounds.top) / bounds.height) * 100)
    });
  }

  return (
    <button
      className={`board ${targeting ? "board--targeting" : ""}`}
      onClick={handleClick}
      type="button"
      aria-label={targeting ? "Choose movement target" : "Tactical map"}
    >
      <span className="river" />
      <span className="objective" aria-hidden="true">◇</span>
      {units
        .filter((unit) => unit.visible && unit.alive)
        .map((unit) => (
          <span
            key={unit.id}
            className={[
              "unit",
              `unit--${unit.team}`,
              unit.id === controlledUnitId ? "unit--controlled" : ""
            ].join(" ")}
            style={{ left: `${unit.position.x}%`, top: `${unit.position.y}%` }}
            title={`${unit.role}: ${unit.hp}/${unit.maxHp} HP`}
          >
            <span className="unit__role">{unit.role.slice(0, 1).toUpperCase()}</span>
            <span className="unit__health">
              <span style={{ width: `${Math.max(0, (unit.hp / unit.maxHp) * 100)}%` }} />
            </span>
          </span>
        ))}
      {target && (
        <span
          className="target"
          style={{ left: `${target.x}%`, top: `${target.y}%` }}
          aria-hidden="true"
        />
      )}
      {targeting && <span className="board__hint">Tap the map to set a route</span>}
    </button>
  );
}

