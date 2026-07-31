import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TacticalBoard } from "./TacticalBoard";

const units = [
  { id: "blue", team: "blue" as const, role: "carry", position: { x: 20, y: 20 }, hp: 80, maxHp: 100, cooldownTurns: 0, visible: true, alive: true },
  { id: "hidden", team: "red" as const, role: "mage", position: { x: 80, y: 80 }, hp: 80, maxHp: 100, cooldownTurns: 0, visible: false, alive: true }
];

describe("TacticalBoard", () => {
  it("does not render hidden enemies", () => {
    render(<TacticalBoard units={units} controlledUnitId="blue" targeting={false} onTarget={vi.fn()} />);
    expect(screen.getByTitle(/carry/i)).toBeInTheDocument();
    expect(screen.queryByTitle(/mage/i)).not.toBeInTheDocument();
  });

  it("accepts a map target only while targeting", () => {
    const onTarget = vi.fn();
    const { rerender } = render(<TacticalBoard units={units} controlledUnitId="blue" targeting={false} onTarget={onTarget} />);
    fireEvent.click(screen.getByRole("button", { name: "Tactical map" }));
    expect(onTarget).not.toHaveBeenCalled();
    rerender(<TacticalBoard units={units} controlledUnitId="blue" targeting onTarget={onTarget} />);
    fireEvent.click(screen.getByRole("button", { name: /choose movement/i }), { clientX: 0, clientY: 0 });
    expect(onTarget).toHaveBeenCalledOnce();
  });
});

