import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MomentSummary } from "../types";
import { MainMenu } from "./MainMenu";

afterEach(cleanup);

const tutorialMoment: MomentSummary = {
  id: "objective-steal-742",
  slug: "river-pit-steal",
  title: "The River Pit Steal",
  description: "Secure the objective or escape.",
  map: "Synthetic Rift",
  category: "objective-contest",
  skillLevel: "advanced",
  reasonTags: ["objective-steal"],
  highlightScore: 0.83
};

describe("MainMenu", () => {
  it("routes the tutorial and guide without exposing runtime telemetry", () => {
    const openTutorial = vi.fn();
    const openGuide = vi.fn();
    const openScenario = vi.fn();

    render(
      <MainMenu
        moments={[tutorialMoment]}
        onOpenTutorial={openTutorial}
        onOpenGuide={openGuide}
        onOpenScenario={openScenario}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Tutorial" }));
    fireEvent.click(screen.getByRole("button", { name: "Open Beginner Guide" }));

    expect(openTutorial).toHaveBeenCalledOnce();
    expect(openGuide).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: /telemetry/i })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Fixed scenario library" })).toBeInTheDocument();
    expect(screen.getByText("1 authored lesson")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /The River Pit Steal/i }));
    expect(openScenario).toHaveBeenCalledWith(tutorialMoment);
  });
});
