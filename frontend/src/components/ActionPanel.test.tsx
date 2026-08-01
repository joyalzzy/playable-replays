import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ActionPanel } from "./ActionPanel";

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

  it("offers dodge and outplay with descriptive labels", () => {
    render(
      <ActionPanel
        legalActions={["dodge", "outplay"]}
        selected="dodge"
        disabled={false}
        onSelect={vi.fn()}
        onCommit={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: /dodge.*incoming skillshot/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /outplay.*high-risk mechanical/i })).toBeInTheDocument();
  });
});
