import { useEffect, useRef, useState, type CSSProperties, type MouseEvent } from "react";
import type { ObjectiveState, Point, Projectile, TerrainFeature, Turret, Unit } from "../types";

const laneIDs = ["top-lane", "middle-lane", "bottom-lane"] as const;
const laneLayers = ["shadow", "verge", "edge", "stones", "wear"] as const;

function roleClass(role: string) {
  return role.toLowerCase().replaceAll(/[^a-z0-9-]/g, "");
}

function projectileAngle(projectile: Projectile) {
  const deltaX = projectile.target.x - projectile.position.x;
  const deltaY = projectile.target.y - projectile.position.y;
  return Math.atan2(deltaY, deltaX) * 180 / Math.PI;
}

// The supplied sprites point toward the upper-right (-45 degrees in screen
// coordinates), so add 45 degrees to align the arrow with a movement vector.
export function movementFacingAngle(team: Unit["team"], previous?: Point, current?: Point) {
  if (previous && current) {
    const deltaX = current.x - previous.x;
    const deltaY = current.y - previous.y;
    if (Math.hypot(deltaX, deltaY) > 0.001) {
      return Math.atan2(deltaY, deltaX) * 180 / Math.PI + 45;
    }
  }
  return team === "blue" ? 0 : 180;
}

type Props = {
  units: Unit[];
  terrain: TerrainFeature[];
  objective?: ObjectiveState;
  turrets: Turret[];
  projectiles: Projectile[];
  controlledUnitId: string;
  unknownEnemyCount: number;
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
  kind,
  projectedPosition,
  viewportWidth,
  viewportHeight
}: {
  unit: Unit;
  kind: "attack" | "move";
  projectedPosition: Point;
  viewportWidth: number;
  viewportHeight: number;
}) {
  const range = kind === "attack" ? unit.attackRange : unit.moveRange;
  return (
    <span
      className={`range-indicator range-indicator--${kind}`}
      style={{
        left: `${projectedPosition.x}%`,
        top: `${projectedPosition.y}%`,
        width: `${(Math.max(0, range) * 2 / viewportWidth) * 100}%`,
        height: `${(Math.max(0, range) * 2 / viewportHeight) * 100}%`
      }}
      role="img"
      aria-label={`${kind === "attack" ? "Attack" : "Movement"} range for ${unit.class}: ${range} map units`}
    />
  );
}

export function TacticalBoard({
  units,
  terrain,
  objective,
  turrets,
  projectiles,
  controlledUnitId,
  unknownEnemyCount,
  target,
  targeting,
  onTarget
}: Props) {
  const [hoverPoint, setHoverPoint] = useState<Point>();
  const [selectedUnitId, setSelectedUnitId] = useState(controlledUnitId);
  const previousPositions = useRef(new Map<string, Point>());
  const facingAngles = useRef(new Map<string, number>());
  const visibleUnits = units.filter((unit) => unit.visible && unit.alive);
  const controlledUnit = visibleUnits.find((unit) => unit.id === controlledUnitId);
  const selectedUnit = visibleUnits.find((unit) => unit.id === selectedUnitId) ?? controlledUnit;
  const viewportWidth = 100;
  const viewportHeight = 100;

  useEffect(() => {
    setSelectedUnitId(controlledUnitId);
  }, [controlledUnitId]);

  function projectPoint(point: Point) {
    return {
      x: clamp(point.x, 0, 100),
      y: clamp(point.y, 0, 100)
    };
  }

  function pointFromEvent(event: MouseEvent<HTMLButtonElement>): Point {
    const bounds = event.currentTarget.getBoundingClientRect();
    const screenX = bounds.width > 0 ? (event.clientX - bounds.left) / bounds.width : 0;
    const screenY = bounds.height > 0 ? (event.clientY - bounds.top) / bounds.height : 0;
    const x = screenX * viewportWidth;
    const y = screenY * viewportHeight;
    return {
      x: Math.min(100, Math.max(0, Math.round(x))),
      y: Math.min(100, Math.max(0, Math.round(y)))
    };
  }

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    if (!targeting || !controlledUnit) return;
    onTarget(clampTargetToMoveRange(pointFromEvent(event), controlledUnit.position, controlledUnit.moveRange));
  }

  return (
    <section className="tactical-board" aria-label="Tactical board">
      <div className={`board ${targeting ? "board--targeting" : ""}`}>
        <button
          className="board__surface"
          onClick={handleClick}
          onMouseMove={(event) => setHoverPoint(pointFromEvent(event))}
          onMouseLeave={() => setHoverPoint(undefined)}
          type="button"
          aria-label={targeting ? "Choose movement target" : "Tactical map"}
        />
      <svg
        className="lane-network"
        viewBox="0 0 160 100"
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        <defs>
          <pattern id="lane-stone-pattern" width="7" height="7" patternUnits="userSpaceOnUse">
            <rect width="7" height="7" fill="#626758" />
            <path d="M0 3.5H7M3.5 0V3.5M1.75 3.5V7M5.25 3.5V7" stroke="#808372" strokeWidth="0.34" />
            <path d="M0 0H7V7H0Z" fill="none" stroke="#3d443b" strokeWidth="0.42" />
          </pattern>
          <path id="top-lane" d="M25.6 81.7 L18 68 Q12 38 41 19 Q75 1 121 9 L134.4 18.3" />
          <path id="middle-lane" d="M9.6 91 L150.4 9" />
          <path id="bottom-lane" d="M25.6 81.7 L39 91 Q84 101 121 78 Q147 58 142 31 L134.4 18.3" />
        </defs>

        {laneLayers.map((layer) => (
          <g key={layer} className={`lane-route lane-route-layer--${layer}`}>
            {laneIDs.map((laneID) => (
              <use
                key={laneID}
                href={`#${laneID}`}
                className={`lane-route__${layer} lane-route--${laneID}`}
              />
            ))}
          </g>
        ))}

        <g className="lane-gate lane-gate--blue">
          <circle cx="22.9" cy="77.1" r="1.3" className="lane-gate__pylon" />
          <circle cx="28.3" cy="86.3" r="1.3" className="lane-gate__pylon" />
        </g>
        <g className="lane-gate lane-gate--red">
          <circle cx="131.7" cy="13.7" r="1.3" className="lane-gate__pylon" />
          <circle cx="137.1" cy="22.9" r="1.3" className="lane-gate__pylon" />
        </g>

        <g className="lane-base lane-base--blue">
          <circle cx="9.6" cy="91" r="7.6" className="lane-base__halo" />
          <circle cx="9.6" cy="91" r="5.2" className="lane-base__platform" />
          <path d="M5.4 91 L9.6 86.8 L13.8 91 L9.6 95.2 Z" className="lane-base__rune" />
        </g>
        <g className="lane-base lane-base--red">
          <circle cx="150.4" cy="9" r="7.6" className="lane-base__halo" />
          <circle cx="150.4" cy="9" r="5.2" className="lane-base__platform" />
          <path d="M146.2 9 L150.4 4.8 L154.6 9 L150.4 13.2 Z" className="lane-base__rune" />
        </g>
      </svg>
      <span className="map-focus">Full map</span>
      {terrain.map((feature) => (
        <span
          key={feature.id}
          className={`terrain terrain--${feature.kind}`}
          data-label={feature.label}
          style={{
            left: `${projectPoint(feature.position).x}%`,
            top: `${projectPoint(feature.position).y}%`,
            width: `${(feature.radius * 2 / viewportWidth) * 100}%`,
            height: `${(feature.radius * 2 / viewportHeight) * 100}%`
          }}
          title={`${feature.label}${feature.blocksVision ? ": blocks long-range vision" : ""}`}
          aria-hidden="true"
        />
      ))}
      {unknownEnemyCount > 0 && controlledUnit && (
        <span
          className="fog-of-war"
          style={{
            "--vision-x": `${projectPoint(controlledUnit.position).x}%`,
            "--vision-y": `${projectPoint(controlledUnit.position).y}%`
          } as CSSProperties}
          aria-hidden="true"
        />
      )}
      {objective && (
        <span
          className={`objective-zone objective-zone--${objective.status}`}
          style={{
            left: `${projectPoint(objective.position).x}%`,
            top: `${projectPoint(objective.position).y}%`,
            width: `${(objective.radius * 2 / viewportWidth) * 100}%`,
            height: `${(objective.radius * 2 / viewportHeight) * 100}%`
          }}
          title={`${objective.label}: blue ${objective.blueProgress}/${objective.requiredProgress}, red ${objective.redProgress}/${objective.requiredProgress}`}
        >
          <span className="objective-zone__pit" aria-hidden="true">
            <span className="objective-zone__core" />
          </span>
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
      {turrets.map((turret) => {
        const health = `, ${turret.hp} of ${turret.maxHp} health`;
        const healthPercent = turret.maxHp > 0
          ? clamp((turret.hp / turret.maxHp) * 100, 0, 100)
          : 0;
        return (
          <span
            key={turret.id}
            className={`turret turret--${turret.team}${turret.alive ? "" : " turret--destroyed"}`}
            style={{ left: `${projectPoint(turret.position).x}%`, top: `${projectPoint(turret.position).y}%` }}
            role="img"
            aria-label={`${turret.team} ${turret.lane} lane turret${turret.alive ? "" : ", destroyed"}${health}`}
            title={`${turret.team} ${turret.lane} lane turret${health}`}
          >
            <span className="turret__crown" aria-hidden="true" />
            <span className="turret__health" aria-hidden="true">
              <span style={{ width: `${healthPercent}%` }} />
            </span>
          </span>
        );
      })}
      {selectedUnit && (
        <RangeIndicator
          unit={selectedUnit}
          kind="attack"
          projectedPosition={projectPoint(selectedUnit.position)}
          viewportWidth={viewportWidth}
          viewportHeight={viewportHeight}
        />
      )}
      {targeting && controlledUnit && (
        <RangeIndicator
          unit={controlledUnit}
          kind="move"
          projectedPosition={projectPoint(controlledUnit.position)}
          viewportWidth={viewportWidth}
          viewportHeight={viewportHeight}
        />
      )}
      {visibleUnits.map((unit) => {
        const selected = unit.id === selectedUnit?.id;
        const previousPosition = previousPositions.current.get(unit.id);
        const moved = previousPosition && Math.hypot(
          unit.position.x - previousPosition.x,
          unit.position.y - previousPosition.y
        ) > 0.001;
        const facing = moved
          ? movementFacingAngle(unit.team, previousPosition, unit.position)
          : (facingAngles.current.get(unit.id) ?? movementFacingAngle(unit.team));
        previousPositions.current.set(unit.id, { ...unit.position });
        facingAngles.current.set(unit.id, facing);
        return (
          <button
            key={unit.id}
            className={[
              "unit",
              `unit--${unit.team}`,
              `unit--role-${roleClass(unit.role)}`,
              `unit--class-${unit.class}`,
              unit.id === controlledUnitId ? "unit--controlled" : "",
              unit.guarded ? "unit--guarded" : "",
              selected ? "unit--selected" : ""
            ].filter(Boolean).join(" ")}
            style={{
              left: `${projectPoint(unit.position).x}%`,
              top: `${projectPoint(unit.position).y}%`,
              "--unit-facing": `${facing}deg`
            } as CSSProperties}
            title={`${unit.class} ${unit.role}: ${unit.hp}/${unit.maxHp} HP · ${unit.moveRange} movement · ${unit.attackRange} attack range`}
            type="button"
            aria-label={`${unit.team} ${unit.class} unit, ${unit.hp} of ${unit.maxHp} health${unit.id === controlledUnitId ? ", controlled" : ""}`}
            aria-pressed={selected}
            onClick={() => setSelectedUnitId(unit.id)}
            onFocus={() => setSelectedUnitId(unit.id)}
          >
            <span className="unit__sprite" aria-hidden="true" />
            <span className="unit__team-dot" aria-hidden="true" />
            {unit.shield > 0 && <span className="unit__shield">+{unit.shield}</span>}
            <span className="unit__health">
              <span style={{ width: `${Math.max(0, (unit.hp / unit.maxHp) * 100)}%` }} />
            </span>
            <span className="unit__class-badge" aria-hidden="true">{unit.class}</span>
          </button>
        );
      })}
      {projectiles.map((projectile) => (
        <span
          key={projectile.id}
          className={`projectile projectile--${projectile.team}`}
          style={{
            left: `${projectPoint(projectile.position).x}%`,
            top: `${projectPoint(projectile.position).y}%`,
            "--projectile-angle": `${projectileAngle(projectile)}deg`
          } as CSSProperties}
          role="img"
          aria-label={`${projectile.team} marksman projectile from ${projectile.sourceUnitId || "unknown source"} targeting ${projectile.targetUnitId}, ${projectile.damage} damage`}
          title={`Heavy marksman projectile · ${projectile.damage} damage`}
        >
          <span aria-hidden="true" />
        </span>
      ))}
      {target && (
        <span
          className="target"
          style={{ left: `${projectPoint(target).x}%`, top: `${projectPoint(target).y}%` }}
          aria-label={`Selected movement target: X ${target.x}, Y ${target.y}`}
        >
          <span className="target__coordinates">X {target.x} · Y {target.y}</span>
        </span>
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
              {selectedUnit.team} team · {selectedUnit.role}
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
