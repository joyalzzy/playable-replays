import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ReplayHelp } from "./ReplayHelp";

afterEach(cleanup);

describe("ReplayHelp", () => {
  it("explains the replay loop and every controlled-character command", () => {
    render(<ReplayHelp />);

    const trigger = screen.getByRole("button", { name: "How replay decisions work" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    fireEvent.click(trigger);

    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("heading", { name: "How the replay works" })).toBeInTheDocument();
    expect(screen.getByText(/blue character with the gold ring/i)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "What each choice does" })).toBeInTheDocument();
    expect(screen.getByText("Move")).toBeInTheDocument();
    expect(screen.getByText("Hold")).toBeInTheDocument();
    expect(screen.getByText("Contest")).toBeInTheDocument();
    expect(screen.getByText("Retreat")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Separate projectile response" })).toBeInTheDocument();
    expect(screen.getByText("Dodge · two charges")).toBeInTheDocument();
  });

  it("closes with Escape and returns focus to the help button", () => {
    render(<ReplayHelp />);

    const trigger = screen.getByRole("button", { name: "How replay decisions work" });
    fireEvent.click(trigger);
    fireEvent.keyDown(window, { key: "Escape" });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("identifies every character, status, and map symbol used by the replay", () => {
    render(<ReplayHelp />);
    fireEvent.click(screen.getByRole("button", { name: "How replay decisions work" }));

    expect(screen.getByRole("heading", { name: "Character role icons" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Team and status indicators" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Map and terrain symbols" })).toBeInTheDocument();

    const legendEntries = [
      "Carry",
      "Support",
      "Jungler",
      "Mage",
      "Tank",
      "Assassin",
      "Your character",
      "Ally",
      "Enemy",
      "Guarded",
      "Shield",
      "Health",
      "Limited vision",
      "Objective core",
      "Safe zone",
      "Brush",
      "Wall",
      "River",
      "Move target",
      "Team base",
      "Base gate",
      "Lane turret",
      "Marksman projectile"
    ];

    for (const entry of legendEntries) {
      expect(screen.getByRole("article", { name: entry })).toBeInTheDocument();
    }
  });
});
