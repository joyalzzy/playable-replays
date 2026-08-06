import { useEffect, useState, type MouseEvent } from "react";
import type { Point, Unit } from "../types";

type Props = {
  units: Unit[];
  controlledUnitId: string;
  target?: Point;
  targeting: boolean;
  onTarget: (point: Point) => void;
};

const clamp = (value: number, minimum: number, maximum: number) =>
  Math.min(maximum, Math.max(minimum, value));

export function clampTargetToMoveRange(
  point: Point,
  origin: Point,
  moveRange: number
): Point {
  const bounded = {
    x: clamp(point.x, 0, 100),
    y: clamp(point.y, 0, 100)
  };
  const range = Math.max(0, moveRange);
  const deltaX = bounded.x - origin.x;
  const deltaY = bounded.y - origin.y;
  const distance = Math.hypot(deltaX, deltaY);

  if (distance === 0 || distance <= range) return bounded;

  const scale = range / distance;
  return {
    x: clamp(origin.x + deltaX * scale, 0, 100),
    y: clamp(origin.y + deltaY * scale, 0, 100)
  };
}

function RangeIndicator({
  unit,
  kind
}: {
  unit: Unit;
  kind: "attack" | "move";
}) {
  const range = kind === "attack" ? unit.attackRange : unit.moveRange;
  return (
    <span
      className={`range-indicator range-indicator--${kind}`}
      style={{
        left: `${unit.position.x}%`,
        top: `${unit.position.y}%`,
        width: `${Math.max(0, range) * 2}%`,
        height: `${Math.max(0, range) * 2}%`
      }}
      role="img"
      aria-label={`${kind === "attack" ? "Attack" : "Movement"} range for ${unit.class}: ${range} map units`}
    />
  );
}

export function TacticalBoard({
  units,
  controlledUnitId,
  target,
  targeting,
  onTarget
}: Props) {
  const [selectedUnitId, setSelectedUnitId] = useState(controlledUnitId);
  const visibleUnits = units.filter((unit) => unit.visible && unit.alive);
  const controlledUnit = visibleUnits.find((unit) => unit.id === controlledUnitId);
  const selectedUnit =
    visibleUnits.find((unit) => unit.id === selectedUnitId) ?? controlledUnit;

  useEffect(() => {
    setSelectedUnitId(controlledUnitId);
  }, [controlledUnitId]);

  function handleMapClick(event: MouseEvent<HTMLButtonElement>) {
    if (!targeting || !controlledUnit) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    if (bounds.width <= 0 || bounds.height <= 0) return;

    const requestedPoint = {
      x: ((event.clientX - bounds.left) / bounds.width) * 100,
      y: ((event.clientY - bounds.top) / bounds.height) * 100
    };
    onTarget(
      clampTargetToMoveRange(
        requestedPoint,
        controlledUnit.position,
        controlledUnit.moveRange
      )
    );
  }

  return (
    <section className="tactical-board" aria-label="Tactical board">
      <div className={`board ${targeting ? "board--targeting" : ""}`}>
        <button
          className="board__surface"
          onClick={handleMapClick}
          type="button"
          aria-label={targeting ? "Choose movement target" : "Tactical map"}
        />
        <span className="river" aria-hidden="true" />
        <span className="objective" aria-hidden="true">◇</span>

        {selectedUnit && <RangeIndicator unit={selectedUnit} kind="attack" />}
        {targeting && controlledUnit && (
          <RangeIndicator unit={controlledUnit} kind="move" />
        )}

        {visibleUnits.map((unit) => {
          const isSelected = unit.id === selectedUnit?.id;
          return (
            <button
              key={unit.id}
              className={[
                "unit",
                `unit--${unit.team}`,
                `unit--class-${unit.class}`,
                unit.id === controlledUnitId ? "unit--controlled" : "",
                isSelected ? "unit--selected" : ""
              ].filter(Boolean).join(" ")}
              style={{ left: `${unit.position.x}%`, top: `${unit.position.y}%` }}
              type="button"
              aria-label={`${unit.team} ${unit.class} unit, ${unit.hp} of ${unit.maxHp} health${unit.id === controlledUnitId ? ", controlled" : ""}`}
              aria-pressed={isSelected}
              onClick={() => setSelectedUnitId(unit.id)}
              onFocus={() => setSelectedUnitId(unit.id)}
            >
              <span className="unit__marker" aria-hidden="true">
                <span className="unit__role">{unit.class.slice(0, 1).toUpperCase()}</span>
              </span>
              <span className="unit__health" aria-hidden="true">
                <span style={{ width: `${Math.max(0, (unit.hp / unit.maxHp) * 100)}%` }} />
              </span>
              <span className="unit__class-badge" aria-hidden="true">{unit.class}</span>
            </button>
          );
        })}

        {target && (
          <span
            className="target"
            style={{ left: `${target.x}%`, top: `${target.y}%` }}
            aria-hidden="true"
          />
        )}
        {targeting && controlledUnit && (
          <span className="board__hint">
            Choose a point within {controlledUnit.moveRange} map units
          </span>
        )}
      </div>

      {selectedUnit && (
        <section
          className={`unit-inspector unit-inspector--${selectedUnit.class}`}
          aria-label="Selected unit details"
          aria-live="polite"
        >
          <div>
            <span className="unit-inspector__label">Selected unit</span>
            <strong>{selectedUnit.class}</strong>
            <span className={`team-label team-label--${selectedUnit.team}`}>
              {selectedUnit.team} team
            </span>
          </div>
          <dl>
            <div>
              <dt>Health</dt>
              <dd>{selectedUnit.hp} / {selectedUnit.maxHp} HP</dd>
            </div>
            <div>
              <dt>Move per frame</dt>
              <dd>{selectedUnit.moveRange} map units</dd>
            </div>
            <div>
              <dt>Attack range</dt>
              <dd>{selectedUnit.attackRange} map units</dd>
            </div>
          </dl>
        </section>
      )}
    </section>
  );
}
