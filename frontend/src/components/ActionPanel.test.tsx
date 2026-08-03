import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ActionPanel } from "./ActionPanel";

afterEach(cleanup);

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
});
