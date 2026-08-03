import { describe, expect, it } from "vitest";
import { scenarioOptionLabel, sortMomentsByDifficulty } from "./scenarioOrder";
import type { MomentSummary } from "./types";

function moment(title: string, skillLevel: MomentSummary["skillLevel"]): MomentSummary {
  return {
    id: title,
    slug: title.toLowerCase().replaceAll(" ", "-"),
    title,
    description: "Test scenario",
    map: "Test Rift",
    category: "positioning",
    skillLevel,
    reasonTags: [],
    highlightScore: 0.5
  };
}

describe("scenario selector ordering", () => {
  it("sorts beginner, intermediate, then advanced while preserving authored order within a level", () => {
    const moments = [
      moment("Advanced One", "advanced"),
      moment("Beginner One", "beginner"),
      moment("Intermediate One", "intermediate"),
      moment("Beginner Two", "beginner")
    ];

    expect(sortMomentsByDifficulty(moments).map((item) => item.title)).toEqual([
      "Beginner One",
      "Beginner Two",
      "Intermediate One",
      "Advanced One"
    ]);
    expect(moments.map((item) => item.title)).toEqual([
      "Advanced One",
      "Beginner One",
      "Intermediate One",
      "Beginner Two"
    ]);
  });

  it("puts a capitalized difficulty before the scenario name", () => {
    expect(scenarioOptionLabel(moment("Support Anchor Line", "beginner"))).toBe(
      "(Beginner) Support Anchor Line"
    );
  });
});
