import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Session } from "../types";
import { OutcomeDebrief } from "./OutcomeDebrief";

afterEach(cleanup);

const terminalSession: Session = {
  id: "s1",
  momentId: "m1",
  controlledUnitId: "blue",
  scenarioGoal: "Secure the objective.",
  turn: 2,
  maxTurns: 3,
  status: "won",
  outcomeReason: "Blue secured the core.",
  advantage: 0.8,
  escapeProgress: 0,
  escapeTurnsRequired: 2,
  visibleEnemyCount: 1,
  unknownEnemyCount: 0,
  visionLimited: false,
  terrain: [],
  referenceOutcomes: [
    {
      firstAction: { type: "contest" },
      status: "won",
      turns: 2,
      advantage: 0.8,
      outcomeReason: "Blue secured the core.",
      keyEvents: ["Blue carry hit red carry for 18 damage."]
    }
  ],
  bestCase: {
    status: "won",
    turns: 2,
    advantage: 0.84,
    outcomeReason: "Blue secured the core.",
    method: "Exhaustive deterministic search over all modeled commands.",
    steps: [
      {
        turn: 1,
        action: { type: "contest" },
        reason: "Contest was the only modeled opening that preserved a winning line.",
        advantageBefore: 0.5,
        advantageAfter: 0.65,
        keyEvents: ["Blue carry damaged the nearest threat."],
        alternatives: [
          { action: { type: "contest" }, status: "won", turns: 2, advantage: 0.84, outcomeReason: "Blue secured the core." },
          { action: { type: "hold" }, status: "lost", turns: 3, advantage: 0.2, outcomeReason: "Red secured the core." }
        ]
      },
      {
        turn: 2,
        action: { type: "hold" },
        reason: "Hold protected the carry while objective control completed.",
        advantageBefore: 0.65,
        advantageAfter: 0.84,
        keyEvents: ["Blue secured the core."],
        alternatives: [
          { action: { type: "hold" }, status: "won", turns: 2, advantage: 0.84, outcomeReason: "Blue secured the core." }
        ]
      }
    ]
  },
  legalActions: ["move", "hold", "contest", "retreat"],
  units: [],
  log: [],
  debrief: ["Scenario advantage is not a calibrated win probability."]
};

describe("OutcomeDebrief", () => {
  it("shows causal outcome and post-scenario reference rollouts", () => {
    render(<OutcomeDebrief session={terminalSession} busy={false} onReplay={vi.fn()} />);
    expect(screen.getByRole("heading", { name: /scenario secured/i })).toBeInTheDocument();
    expect(screen.getAllByText("Blue secured the core.")).toHaveLength(2);
    expect(screen.getByText(/reference rollouts · not historical outcomes/i)).toBeInTheDocument();
    expect(screen.getByText(/contest/i)).toBeInTheDocument();
  });

  it("reveals a selectable, reasoned best-case turn sequence", () => {
    render(<OutcomeDebrief session={terminalSession} busy={false} onReplay={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: /explore calculated best case/i }));
    expect(screen.getByText(/only modeled opening that preserved a winning line/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /turn 2: hold/i }));
    expect(screen.getByText(/protected the carry while objective control completed/i)).toBeInTheDocument();
    expect(screen.getByText(/this is a simulation, not a guaranteed match outcome/i)).toBeInTheDocument();
  });
});
