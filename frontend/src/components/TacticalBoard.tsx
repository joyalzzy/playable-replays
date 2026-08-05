import { useState, type CSSProperties, type MouseEvent } from "react";
import { fullMapViewport, type MapViewport } from "../mapViewport";
import { roleMarks } from "../replaySymbols";
import type { ObjectiveState, Point, TerrainFeature, Unit } from "../types";

const laneIDs = ["top-lane", "middle-lane", "bottom-lane"] as const;
const laneLayers = ["shadow", "verge", "edge", "stones", "wear"] as const;

function roleClass(role: string) {
  return role.toLowerCase().replaceAll(/[^a-z0-9-]/g, "");
}

type Props = {
  units: Unit[];
  terrain: TerrainFeature[];
  objective?: ObjectiveState;
  controlledUnitId: string;
  unknownEnemyCount: number;
  viewport?: MapViewport;
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
  viewport,
  target,
  targeting,
  onTarget
}: Props) {
  const [hoverPoint, setHoverPoint] = useState<Point>();
  const controlledUnit = units.find((unit) => unit.id === controlledUnitId);
  const activeViewport = viewport ?? fullMapViewport;
  const viewportWidth = activeViewport.xMax - activeViewport.xMin;
  const viewportHeight = activeViewport.yMax - activeViewport.yMin;

  function projectPoint(point: Point) {
    return {
      x: ((point.x - activeViewport.xMin) / viewportWidth) * 100,
      y: ((point.y - activeViewport.yMin) / viewportHeight) * 100
    };
  }

  function pointFromEvent(event: MouseEvent<HTMLButtonElement>): Point {
    const bounds = event.currentTarget.getBoundingClientRect();
    const screenX = bounds.width > 0 ? (event.clientX - bounds.left) / bounds.width : 0;
    const screenY = bounds.height > 0 ? (event.clientY - bounds.top) / bounds.height : 0;
    const x = activeViewport.xMin + screenX * viewportWidth;
    const y = activeViewport.yMin + screenY * viewportHeight;
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
      <svg
        className="lane-network"
        viewBox={`${activeViewport.xMin * 1.6} ${activeViewport.yMin} ${viewportWidth * 1.6} ${viewportHeight}`}
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
      {viewport && <span className="map-focus">{viewport.label}</span>}
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
      {units
        .filter((unit) => unit.visible && unit.alive)
        .map((unit) => (
          <span
            key={unit.id}
            className={[
              "unit",
              `unit--${unit.team}`,
              `unit--role-${roleClass(unit.role)}`,
              unit.id === controlledUnitId ? "unit--controlled" : "",
              unit.guarded ? "unit--guarded" : ""
            ].join(" ")}
            style={{ left: `${projectPoint(unit.position).x}%`, top: `${projectPoint(unit.position).y}%` }}
            title={`${unit.role}: ${unit.hp}/${unit.maxHp} HP · ${unit.shield} shield · ${unit.attackRange} range`}
          >
            <span className="unit__portrait" aria-hidden="true">
              <span className="unit__role">{roleMarks[unit.role] ?? unit.role.slice(0, 2).toUpperCase()}</span>
            </span>
            <span className="unit__team-dot" aria-hidden="true" />
            {unit.shield > 0 && <span className="unit__shield">+{unit.shield}</span>}
            <span className="unit__health">
              <span style={{ width: `${Math.max(0, (unit.hp / unit.maxHp) * 100)}%` }} />
            </span>
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
      {targeting && <span className="board__hint">Choose a destination within the tactical map</span>}
    </button>
  );
}
