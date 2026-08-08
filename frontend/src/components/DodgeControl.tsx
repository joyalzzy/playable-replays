import { useId } from "react";

type Props = {
  charges: number;
  available: boolean;
  incomingProjectiles: number;
  disabled: boolean;
  onDodge: () => void;
};

export function DodgeControl({
  charges,
  available,
  incomingProjectiles,
  disabled,
  onDodge
}: Props) {
  const statusID = useId();
  const chargeLabel = `${charges} dodge ${charges === 1 ? "charge" : "charges"} remaining`;
  let status = "No projectile can be dodged right now.";

  if (charges <= 0) {
    status = "Both dodge charges have been used for this scenario.";
  } else if (available) {
    status = `${incomingProjectiles} incoming ${incomingProjectiles === 1 ? "projectile is" : "projectiles are"} threatening your character.`;
  }

  return (
    <section className="dodge-control" aria-label="Projectile dodge">
      <div>
        <p className="eyebrow">PROJECTILE RESPONSE</p>
        <strong>{chargeLabel}</strong>
        <span id={statusID} aria-live="polite">{status}</span>
      </div>
      <span className="dodge-control__charges" aria-hidden="true">
        <i className={charges >= 1 ? "is-ready" : ""} />
        <i className={charges >= 2 ? "is-ready" : ""} />
      </span>
      <button
        type="button"
        aria-describedby={statusID}
        onClick={onDodge}
        disabled={disabled || !available || charges <= 0}
      >
        Dodge projectile
      </button>
    </section>
  );
}
