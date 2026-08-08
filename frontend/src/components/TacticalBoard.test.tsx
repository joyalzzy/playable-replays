import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Projectile, Turret, Unit } from "../types";
import { clampTargetToMoveRange, TacticalBoard } from "./TacticalBoard";

afterEach(cleanup);

const units: Unit[] = [
  {
    id: "blue",
    team: "blue",
    role: "tank",
    class: "tank",
    policy: "controlled",
    position: { x: 20, y: 20 },
    hp: 140,
    maxHp: 160,
    moveRange: 7,
    attackRange: 10,
    attackDamage: 18,
    moveSpeed: 7,
    armor: 20,
    visionRange: 34,
    attackCooldown: 2,
    cooldownTurns: 0,
    shield: 0,
    guarded: false,
    visible: true,
    alive: true
  },
  {
    id: "red-mage",
    team: "red",
    role: "mage",
    class: "mage",
    policy: "skirmisher",
    position: { x: 25, y: 20 },
    hp: 55,
    maxHp: 95,
    moveRange: 9,
    attackRange: 24,
    attackDamage: 20,
    moveSpeed: 9,
    armor: 10,
    visionRange: 34,
    attackCooldown: 2,
    cooldownTurns: 0,
    shield: 0,
    guarded: false,
    visible: true,
    alive: true
  },
  {
    id: "hidden",
    team: "red",
    role: "assassin",
    class: "assassin",
    policy: "aggressive",
    position: { x: 80, y: 80 },
    hp: 80,
    maxHp: 100,
    moveRange: 13,
    attackRange: 12,
    attackDamage: 20,
    moveSpeed: 13,
    armor: 10,
    visionRange: 34,
    attackCooldown: 2,
    cooldownTurns: 0,
    shield: 0,
    guarded: false,
    visible: false,
    alive: true
  }
];

const turrets: Turret[] = [
  { id: "blue-top", team: "blue", lane: "top", position: { x: 22, y: 32 }, hp: 100, maxHp: 100, alive: true },
  { id: "blue-middle", team: "blue", lane: "middle", position: { x: 35, y: 65 }, hp: 100, maxHp: 100, alive: true },
  { id: "blue-bottom", team: "blue", lane: "bottom", position: { x: 45, y: 82 }, hp: 100, maxHp: 100, alive: true },
  { id: "red-top", team: "red", lane: "top", position: { x: 55, y: 18 }, hp: 100, maxHp: 100, alive: true },
  { id: "red-middle", team: "red", lane: "middle", position: { x: 65, y: 35 }, hp: 100, maxHp: 100, alive: true },
  { id: "red-bottom", team: "red", lane: "bottom", position: { x: 78, y: 68 }, hp: 100, maxHp: 100, alive: true }
];

const projectiles: Projectile[] = [{
  id: "red-heavy-shot",
  team: "red",
  sourceUnitId: "red-mage",
  targetUnitId: "blue",
  position: { x: 24, y: 20 },
  target: { x: 20, y: 20 },
  damage: 80
}];

describe("TacticalBoard", () => {
  it("does not render hidden enemies", () => {
    const { container } = render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={[]}
        projectiles={[]}
        controlledUnitId="blue"
        unknownEnemyCount={1}
        targeting={false}
        onTarget={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: /blue tank unit/i })).toHaveClass("unit--class-tank");
    expect(screen.queryByRole("button", { name: /assassin unit/i })).not.toBeInTheDocument();
    expect(screen.getByText(/1 enemy contact unaccounted/i)).toBeInTheDocument();
    expect(container.querySelector(".fog-of-war")).toHaveStyle("--vision-x: 20%; --vision-y: 20%");
  });

  it("selects units by keyboard focus and exposes an accessible inspector", () => {
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={[]}
        projectiles={[]}
        controlledUnitId="blue"
        unknownEnemyCount={1}
        targeting={false}
        onTarget={vi.fn()}
      />
    );

    fireEvent.focus(screen.getByRole("button", { name: /red mage unit/i }));
    const inspector = screen.getByRole("region", { name: /selected unit details/i });
    expect(within(inspector).getByText("mage")).toBeInTheDocument();
    expect(within(inspector).getByText("55 / 95 HP")).toBeInTheDocument();
    expect(within(inspector).getByText("9 map units")).toBeInTheDocument();
    expect(within(inspector).getByText("24 map units")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /attack range for mage: 24/i })).toBeInTheDocument();
  });

  it("shows attack and class-aware movement range indicators", () => {
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={[]}
        projectiles={[]}
        controlledUnitId="blue"
        unknownEnemyCount={1}
        targeting
        onTarget={vi.fn()}
      />
    );

    expect(screen.getByRole("img", { name: /attack range for tank: 10/i })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /movement range for tank: 7/i })).toBeInTheDocument();
    expect(screen.getByText(/within 7 map units/i)).toBeInTheDocument();
  });

  it("selects a highlighted in-range enemy as the contest target", () => {
    const onAttackTarget = vi.fn();
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={[]}
        projectiles={[]}
        controlledUnitId="blue"
        unknownEnemyCount={1}
        targeting={false}
        onTarget={vi.fn()}
        attackTargetIds={["red-mage"]}
        onAttackTarget={onAttackTarget}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: /red mage unit.*selectable attack target/i }));
    expect(onAttackTarget).toHaveBeenCalledWith("red-mage");
    expect(screen.getByText(/select a highlighted enemy within 10 attack range/i)).toBeInTheDocument();
  });

  it("clamps a selected map point to the controlled class move range", () => {
    const onTarget = vi.fn();
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={[]}
        projectiles={[]}
        controlledUnitId="blue"
        unknownEnemyCount={0}
        targeting
        onTarget={onTarget}
      />
    );
    const board = screen.getByRole("button", { name: /choose movement/i });
    vi.spyOn(board, "getBoundingClientRect").mockReturnValue({
      left: 0,
      top: 0,
      width: 100,
      height: 100,
      right: 100,
      bottom: 100,
      x: 0,
      y: 0,
      toJSON: () => undefined
    });

    fireEvent.click(board, { clientX: 90, clientY: 90 });
    expect(onTarget).toHaveBeenCalledWith(
      clampTargetToMoveRange({ x: 90, y: 90 }, { x: 20, y: 20 }, 7)
    );
  });

  it("labels a selected movement target with its coordinates", () => {
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={[]}
        projectiles={[]}
        controlledUnitId="blue"
        unknownEnemyCount={0}
        targeting
        target={{ x: 24, y: 22 }}
        onTarget={vi.fn()}
      />
    );
    expect(screen.getByLabelText("Selected movement target: X 24, Y 22")).toBeInTheDocument();
    expect(screen.getByText("X 24 · Y 22")).toBeInTheDocument();
  });

  it("renders the full map with all server-supplied turrets and projectiles", () => {
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        turrets={turrets}
        projectiles={projectiles}
        controlledUnitId="blue"
        unknownEnemyCount={0}
        targeting={false}
        onTarget={vi.fn()}
      />
    );

    expect(screen.getByText("Full map")).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: /lane turret/i })).toHaveLength(6);
    expect(screen.getByRole("img", { name: /red marksman projectile.*80 damage/i })).toBeInTheDocument();
  });
});
