import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Unit } from "../types";
import { TacticalBoard } from "./TacticalBoard";

const units: Unit[] = [
  {
    id: "blue",
    team: "blue",
    role: "frontline",
    class: "tank",
    position: { x: 20, y: 20 },
    hp: 120,
    maxHp: 160,
    moveRange: 7,
    attackRange: 10,
    cooldownTurns: 0,
    visible: true,
    alive: true
  },
  {
    id: "red",
    team: "red",
    role: "mid",
    class: "mage",
    position: { x: 70, y: 60 },
    hp: 55,
    maxHp: 95,
    moveRange: 9,
    attackRange: 24,
    cooldownTurns: 0,
    visible: true,
    alive: true
  },
  {
    id: "hidden",
    team: "red",
    role: "carry",
    class: "marksman",
    position: { x: 80, y: 80 },
    hp: 80,
    maxHp: 90,
    moveRange: 11,
    attackRange: 28,
    cooldownTurns: 0,
    visible: false,
    alive: true
  }
];

describe("TacticalBoard", () => {
  it("shows visible unit classes without leaking hidden enemies", () => {
    render(
      <TacticalBoard
        units={units}
        controlledUnitId="blue"
        targeting={false}
        onTarget={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: /blue tank unit/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /red mage unit/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /red marksman unit/i })).not.toBeInTheDocument();
    expect(screen.queryByText("marksman")).not.toBeInTheDocument();
  });

  it("selects units by keyboard focus and exposes an accessible inspector", () => {
    render(
      <TacticalBoard
        units={units}
        controlledUnitId="blue"
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
        controlledUnitId="blue"
        targeting
        onTarget={vi.fn()}
      />
    );

    expect(screen.getByRole("img", { name: /attack range for tank: 10/i })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /movement range for tank: 7/i })).toBeInTheDocument();
    expect(screen.getByText(/within 7 map units/i)).toBeInTheDocument();
  });

  it("clamps a map target to the controlled class move range", () => {
    const onTarget = vi.fn();
    render(
      <TacticalBoard
        units={units}
        controlledUnitId="blue"
        targeting
        onTarget={onTarget}
      />
    );
    const surface = screen.getByRole("button", { name: /choose movement target/i });
    vi.spyOn(surface, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100,
      toJSON: () => ({})
    });

    fireEvent.click(surface, { clientX: 100, clientY: 20 });

    expect(onTarget).toHaveBeenCalledOnce();
    expect(onTarget).toHaveBeenCalledWith({ x: 27, y: 20 });
  });

  it("does not accept a map target outside move mode", () => {
    const onTarget = vi.fn();
    render(
      <TacticalBoard
        units={units}
        controlledUnitId="blue"
        targeting={false}
        onTarget={onTarget}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Tactical map" }));
    expect(onTarget).not.toHaveBeenCalled();
    expect(screen.queryByRole("img", { name: /movement range/i })).not.toBeInTheDocument();
  });
});
