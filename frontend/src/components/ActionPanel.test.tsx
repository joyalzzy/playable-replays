import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Unit } from "../types";
import { ActionPanel } from "./ActionPanel";

afterEach(cleanup);

const contestTarget: Unit = {
  id: "red-mage", team: "red", role: "enemy mage", class: "mage", policy: "aggressive",
  position: { x: 30, y: 20 }, hp: 60, maxHp: 95, moveRange: 9, attackRange: 24,
  attackDamage: 20, moveSpeed: 9, armor: 10, visionRange: 34, attackCooldown: 2,
  cooldownTurns: 0, shield: 0, guarded: false, visible: true, alive: true
};

describe("ActionPanel", () => {
  it("requires a target before committing move", () => {
    render(
      <ActionPanel
        legalActions={["move", "hold"]}
        selected="move"
        disabled={false}
        onSelect={vi.fn()}
        onCommit={vi.fn()}
      />
    );
    expect(screen.getByRole("button", { name: /choose a point/i })).toBeDisabled();
  });

  it("commits a legal action", () => {
    const commit = vi.fn();
    render(
      <ActionPanel
        legalActions={["move", "hold"]}
        selected="hold"
        disabled={false}
        onSelect={vi.fn()}
        onCommit={commit}
      />
    );
    fireEvent.click(screen.getByRole("button", { name: /commit decision/i }));
    expect(commit).toHaveBeenCalledOnce();
  });

  it("requires and announces an in-range contest target", () => {
    const onTargetUnit = vi.fn();
    const commit = vi.fn();
    render(
      <ActionPanel
        legalActions={["contest"]}
        selected="contest"
        contestTargets={[contestTarget]}
        disabled={false}
        onSelect={vi.fn()}
        onTargetUnit={onTargetUnit}
        onCommit={commit}
      />
    );

    expect(screen.getByRole("button", { name: /select an enemy in attack range/i })).toBeDisabled();
    fireEvent.change(screen.getByRole("combobox", { name: /enemy inside controlled attack range/i }), {
      target: { value: "red-mage" }
    });
    expect(onTargetUnit).toHaveBeenCalledWith("red-mage");
  });

  it("commits the selected contest target", () => {
    const commit = vi.fn();
    render(
      <ActionPanel
        legalActions={["contest"]}
        selected="contest"
        contestTargets={[contestTarget]}
        selectedTargetUnitId="red-mage"
        disabled={false}
        onSelect={vi.fn()}
        onCommit={commit}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: /attack enemy mage/i }));
    expect(commit).toHaveBeenCalledOnce();
  });

  it("shows the selected movement coordinates before committing", () => {
    render(
      <ActionPanel
        legalActions={["move", "hold"]}
        selected="move"
        target={{ x: 42, y: 68 }}
        disabled={false}
        onSelect={vi.fn()}
        onCommit={vi.fn()}
      />
    );
    expect(screen.getByText("Selected movement point")).toBeInTheDocument();
    expect(screen.getByText("X 42 · Y 68")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Commit move to X 42, Y 68" })).toBeEnabled();
  });

  it("offers exactly the four tactical commands", () => {
    render(
      <ActionPanel
        legalActions={["move", "hold", "contest", "retreat"]}
        selected="hold"
        disabled={false}
        onSelect={vi.fn()}
        onCommit={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: /move.*reposition/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /hold.*brace/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /contest.*visible enemy.*attack range.*attack when ready/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /retreat.*safe zone/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /dodge/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /outplay/i })).not.toBeInTheDocument();
  });
});
