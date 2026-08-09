import { useId } from "react";
import type { Unit } from "../types";

type Props = {
  charges: number;
  available: boolean;
  sources: Unit[];
  targets: Unit[];
  selectedSourceUnitId?: string;
  selectedTargetUnitId?: string;
  disabled: boolean;
  onSource: (unitId: string) => void;
  onTarget: (unitId: string) => void;
  onFire: () => void;
};

export function ProjectileControl({
  charges,
  available,
  sources,
  targets,
  selectedSourceUnitId,
  selectedTargetUnitId,
  disabled,
  onSource,
  onTarget,
  onFire
}: Props) {
  const statusID = useId();
  const selectedSource = sources.find((unit) => unit.id === selectedSourceUnitId);
  const selectedTarget = targets.find((unit) => unit.id === selectedTargetUnitId);
  let status = "Select a marksman and an in-range enemy to queue a shot without advancing the turn.";
  if (charges <= 0) status = "Both player-directed projectile charges have been used.";
  else if (!available) status = "No blue marksman can currently fire at a visible enemy.";

  return (
    <section className="projectile-control" aria-label="Marksman projectile">
      <div className="projectile-control__summary">
        <p className="eyebrow">MARKSMAN SHOT</p>
        <strong>{charges} projectile {charges === 1 ? "charge" : "charges"} remaining</strong>
        <span id={statusID} aria-live="polite">{status}</span>
      </div>
      <span className="projectile-control__charges" aria-hidden="true">
        <i className={charges >= 1 ? "is-ready" : ""} />
        <i className={charges >= 2 ? "is-ready" : ""} />
      </span>
      <div className="projectile-control__selectors">
        <label>
          <span>Marksman source</span>
          <select
            value={selectedSource?.id ?? ""}
            onChange={(event) => onSource(event.target.value)}
            disabled={disabled || charges <= 0 || sources.length === 0}
          >
            <option value="">Select marksman</option>
            {sources.map((unit) => (
              <option key={unit.id} value={unit.id}>
                {unit.role} · cooldown {unit.cooldownTurns}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Enemy target</span>
          <select
            value={selectedTarget?.id ?? ""}
            onChange={(event) => onTarget(event.target.value)}
            disabled={disabled || charges <= 0 || !selectedSource || targets.length === 0}
          >
            <option value="">{selectedSource && targets.length === 0 ? "No enemy in range" : "Select enemy"}</option>
            {targets.map((unit) => (
              <option key={unit.id} value={unit.id}>
                {unit.role} · {unit.class} · {unit.hp}/{unit.maxHp} HP
              </option>
            ))}
          </select>
        </label>
      </div>
      <button
        type="button"
        aria-describedby={statusID}
        onClick={onFire}
        disabled={disabled || !available || charges <= 0 || !selectedSource || !selectedTarget}
      >
        {selectedSource && selectedTarget ? `Fire at ${selectedTarget.role}` : "Fire projectile"}
      </button>
    </section>
  );
}
