import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Unit } from "../types";
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

describe("TacticalBoard", () => {
  it("does not render hidden enemies", () => {
    const { container } = render(
      <TacticalBoard
        units={units}
        terrain={[]}
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

  it("clamps a selected map point to the controlled class move range", () => {
    const onTarget = vi.fn();
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
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

  it("projects focused views while keeping hover coordinates in world space", () => {
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        controlledUnitId="blue"
        unknownEnemyCount={0}
        viewport={{ xMin: 10, xMax: 70, yMin: 20, yMax: 80, label: "Focused view · Test lane" }}
        targeting
        onTarget={vi.fn()}
      />
    );
    const board = screen.getByRole("button", { name: /choose movement/i });
    vi.spyOn(board, "getBoundingClientRect").mockReturnValue({
      left: 0,
      top: 0,
      width: 200,
      height: 100,
      right: 200,
      bottom: 100,
      x: 0,
      y: 0,
      toJSON: () => undefined
    });

    expect(screen.getByText("Focused view · Test lane")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /blue tank unit/i })).toHaveStyle("left: 16.666666666666664%; top: 0%");
    fireEvent.mouseMove(board, { clientX: 100, clientY: 50 });
    expect(screen.getByText("X 40 · Y 50")).toBeInTheDocument();
  });
});
