import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TacticalBoard } from "./TacticalBoard";

afterEach(cleanup);

const units = [
  { id: "blue", team: "blue" as const, role: "carry", policy: "controlled", position: { x: 20, y: 20 }, hp: 80, maxHp: 100, attackRange: 24, attackDamage: 20, moveSpeed: 16, armor: 10, visionRange: 34, attackCooldown: 2, cooldownTurns: 0, shield: 0, guarded: false, visible: true, alive: true },
  { id: "hidden", team: "red" as const, role: "mage", policy: "skirmisher", position: { x: 80, y: 80 }, hp: 80, maxHp: 100, attackRange: 24, attackDamage: 20, moveSpeed: 14, armor: 10, visionRange: 34, attackCooldown: 2, cooldownTurns: 0, shield: 0, guarded: false, visible: false, alive: true }
];

describe("TacticalBoard", () => {
  it("does not render hidden enemies", () => {
    const { container } = render(<TacticalBoard units={units} terrain={[]} controlledUnitId="blue" unknownEnemyCount={1} targeting={false} onTarget={vi.fn()} />);
    expect(screen.getByTitle(/carry/i)).toHaveClass("unit--role-carry");
    expect(screen.getByText("AD")).toBeInTheDocument();
    expect(screen.queryByTitle(/mage/i)).not.toBeInTheDocument();
    expect(screen.getByText(/1 enemy contact unaccounted/i)).toBeInTheDocument();
    expect(container.querySelector(".fog-of-war")).toHaveStyle("--vision-x: 20%; --vision-y: 20%");
  });

  it("accepts a map target only while targeting", () => {
    const onTarget = vi.fn();
    const { rerender } = render(<TacticalBoard units={units} terrain={[]} controlledUnitId="blue" unknownEnemyCount={1} targeting={false} onTarget={onTarget} />);
    fireEvent.click(screen.getByRole("button", { name: "Tactical map" }));
    expect(onTarget).not.toHaveBeenCalled();
    rerender(<TacticalBoard units={units} terrain={[]} controlledUnitId="blue" unknownEnemyCount={1} targeting onTarget={onTarget} />);
    const board = screen.getByRole("button", { name: /choose movement/i });
    vi.spyOn(board, "getBoundingClientRect").mockReturnValue({
      left: 10,
      top: 20,
      width: 200,
      height: 100,
      right: 210,
      bottom: 120,
      x: 10,
      y: 20,
      toJSON: () => undefined
    });
    fireEvent.mouseMove(board, { clientX: 110, clientY: 45 });
    expect(screen.getByText("X 50 · Y 25")).toBeInTheDocument();
    fireEvent.click(board, { clientX: 110, clientY: 45 });
    expect(onTarget).toHaveBeenCalledWith({ x: 50, y: 25 });
    fireEvent.mouseLeave(board);
    expect(screen.getByText("Hover to inspect")).toBeInTheDocument();
  });

  it("labels a selected movement target with its coordinates", () => {
    render(<TacticalBoard units={units} terrain={[]} controlledUnitId="blue" unknownEnemyCount={0} targeting target={{ x: 42, y: 68 }} onTarget={vi.fn()} />);
    expect(screen.getByLabelText("Selected movement target: X 42, Y 68")).toBeInTheDocument();
    expect(screen.getByText("X 42 · Y 68")).toBeInTheDocument();
  });

  it("projects focused views while preserving world-space target coordinates", () => {
    const onTarget = vi.fn();
    render(
      <TacticalBoard
        units={units}
        terrain={[]}
        controlledUnitId="blue"
        unknownEnemyCount={0}
        viewport={{ xMin: 10, xMax: 70, yMin: 20, yMax: 80, label: "Focused view · Test lane" }}
        targeting
        onTarget={onTarget}
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
    expect(screen.getByTitle(/carry/i)).toHaveStyle("left: 16.666666666666664%; top: 0%");
    fireEvent.mouseMove(board, { clientX: 100, clientY: 50 });
    expect(screen.getByText("X 40 · Y 50")).toBeInTheDocument();
    fireEvent.click(board, { clientX: 100, clientY: 50 });
    expect(onTarget).toHaveBeenCalledWith({ x: 40, y: 50 });
  });
});
