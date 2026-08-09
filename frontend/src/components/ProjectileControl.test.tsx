import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Unit } from "../types";
import { ProjectileControl } from "./ProjectileControl";

afterEach(cleanup);

const marksman: Unit = {
  id: "blue-marksman", team: "blue", role: "allied marksman", class: "marksman", policy: "aggressive",
  position: { x: 30, y: 20 }, hp: 90, maxHp: 90, moveRange: 11, attackRange: 28,
  attackDamage: 24, moveSpeed: 11, armor: 14, visionRange: 34, attackCooldown: 2,
  cooldownTurns: 0, shield: 0, guarded: false, visible: true, alive: true
};

const enemy: Unit = {
  ...marksman,
  id: "red-mage", team: "red", role: "enemy mage", class: "mage", hp: 70, maxHp: 95
};

describe("ProjectileControl", () => {
  it("fires from the selected marksman at the selected enemy", () => {
    const onFire = vi.fn();
    render(
      <ProjectileControl
        charges={2}
        available
        sources={[marksman]}
        targets={[enemy]}
        selectedSourceUnitId={marksman.id}
        selectedTargetUnitId={enemy.id}
        disabled={false}
        onSource={vi.fn()}
        onTarget={vi.fn()}
        onFire={onFire}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: /fire at enemy mage/i }));
    expect(onFire).toHaveBeenCalledOnce();
    expect(screen.getByText(/2 projectile charges remaining/i)).toBeInTheDocument();
  });

  it("supports choosing a marksman teammate and target", () => {
    const onSource = vi.fn();
    const onTarget = vi.fn();
    render(
      <ProjectileControl
        charges={2}
        available
        sources={[marksman]}
        targets={[enemy]}
        disabled={false}
        onSource={onSource}
        onTarget={onTarget}
        onFire={vi.fn()}
      />
    );

    fireEvent.change(screen.getByRole("combobox", { name: /marksman source/i }), {
      target: { value: marksman.id }
    });
    fireEvent.change(screen.getByRole("combobox", { name: /enemy target/i }), {
      target: { value: enemy.id }
    });
    expect(onSource).toHaveBeenCalledWith(marksman.id);
    expect(onTarget).toHaveBeenCalledWith(enemy.id);
  });

  it("disables fire when both charges are spent", () => {
    render(
      <ProjectileControl
        charges={0}
        available={false}
        sources={[marksman]}
        targets={[enemy]}
        selectedSourceUnitId={marksman.id}
        selectedTargetUnitId={enemy.id}
        disabled={false}
        onSource={vi.fn()}
        onTarget={vi.fn()}
        onFire={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: /fire at enemy mage/i })).toBeDisabled();
    expect(screen.getByText(/both player-directed projectile charges/i)).toBeInTheDocument();
  });
});
