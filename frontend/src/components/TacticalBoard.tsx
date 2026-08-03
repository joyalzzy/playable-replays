import { useState, type MouseEvent } from "react";
import type { ObjectiveState, Point, TerrainFeature, Unit } from "../types";

type Props = {
  units: Unit[];
  terrain: TerrainFeature[];
  objective?: ObjectiveState;
  controlledUnitId: string;
  unknownEnemyCount: number;
  target?: Point;
  targeting: boolean;
  onTarget: (point: Point) => void;
};

export function TacticalBoard({
  units,
  terrain,
  objective,
  controlledUnitId,
  unknownEnemyCount,
  target,
  targeting,
  onTarget
}: Props) {
  const [hoverPoint, setHoverPoint] = useState<Point>();

  function pointFromEvent(event: MouseEvent<HTMLButtonElement>): Point {
    const bounds = event.currentTarget.getBoundingClientRect();
    const x = bounds.width > 0 ? ((event.clientX - bounds.left) / bounds.width) * 100 : 0;
    const y = bounds.height > 0 ? ((event.clientY - bounds.top) / bounds.height) * 100 : 0;
    return {
      x: Math.min(100, Math.max(0, Math.round(x))),
      y: Math.min(100, Math.max(0, Math.round(y)))
    };
  }

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    if (!targeting) return;
    onTarget(pointFromEvent(event));
  }

  return (
    <button
      className={`board ${targeting ? "board--targeting" : ""}`}
      onClick={handleClick}
      onMouseMove={(event) => setHoverPoint(pointFromEvent(event))}
      onMouseLeave={() => setHoverPoint(undefined)}
      type="button"
      aria-label={targeting ? "Choose movement target" : "Tactical map"}
    >
      <span className="lane lane--one" aria-hidden="true" />
      <span className="lane lane--two" aria-hidden="true" />
      {terrain.map((feature) => (
        <span
          key={feature.id}
          className={`terrain terrain--${feature.kind}`}
          style={{
            left: `${feature.position.x}%`,
            top: `${feature.position.y}%`,
            width: `${feature.radius * 2}%`,
            height: `${feature.radius * 2}%`
          }}
          title={`${feature.label}${feature.blocksVision ? ": blocks long-range vision" : ""}`}
          aria-hidden="true"
        />
      ))}
      {objective && (
        <span
          className={`objective-zone objective-zone--${objective.status}`}
          style={{
            left: `${objective.position.x}%`,
            top: `${objective.position.y}%`,
            width: `${objective.radius * 2}%`,
            height: `${objective.radius * 2}%`
          }}
          title={`${objective.label}: blue ${objective.blueProgress}/${objective.requiredProgress}, red ${objective.redProgress}/${objective.requiredProgress}`}
        >
          <span className="objective-zone__icon">◆</span>
          <span className="objective-zone__score">
            {objective.blueProgress}/{objective.requiredProgress} · {objective.redProgress}/{objective.requiredProgress}
          </span>
        </span>
      )}
      {unknownEnemyCount > 0 && (
        <span className="vision-warning">
          Limited vision · {unknownEnemyCount} enemy {unknownEnemyCount === 1 ? "contact" : "contacts"} unaccounted for
        </span>
      )}
      <span className="coordinate-readout" aria-live="polite">
        <span>Map coordinates</span>
        <strong>
          {hoverPoint ? `X ${hoverPoint.x} · Y ${hoverPoint.y}` : "Hover to inspect"}
        </strong>
      </span>
      {units
        .filter((unit) => unit.visible && unit.alive)
        .map((unit) => (
          <span
            key={unit.id}
            className={[
              "unit",
              `unit--${unit.team}`,
              unit.id === controlledUnitId ? "unit--controlled" : "",
              unit.guarded ? "unit--guarded" : ""
            ].join(" ")}
            style={{ left: `${unit.position.x}%`, top: `${unit.position.y}%` }}
            title={`${unit.role}: ${unit.hp}/${unit.maxHp} HP · ${unit.shield} shield · ${unit.attackRange} range`}
          >
            <span className="unit__role">{unit.role.slice(0, 1).toUpperCase()}</span>
            {unit.shield > 0 && <span className="unit__shield">+{unit.shield}</span>}
            <span className="unit__health">
              <span style={{ width: `${Math.max(0, (unit.hp / unit.maxHp) * 100)}%` }} />
            </span>
          </span>
        ))}
      {target && (
        <span
          className="target"
          style={{ left: `${target.x}%`, top: `${target.y}%` }}
          aria-label={`Selected movement target: X ${target.x}, Y ${target.y}`}
        >
          <span className="target__coordinates">X {target.x} · Y {target.y}</span>
        </span>
      )}
      {targeting && <span className="board__hint">Choose a destination within the tactical map</span>}
    </button>
  );
}
