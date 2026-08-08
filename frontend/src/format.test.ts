import { describe, expect, it } from "vitest";
import { botControlLabel, turnLabel } from "./format";

describe("turnLabel", () => {
  it("labels turns inside the authored guidance horizon", () => {
    expect(turnLabel(3, 5)).toBe("3/5 guided");
  });

  it("keeps live turns understandable beyond that horizon", () => {
    expect(turnLabel(6, 5)).toBe("6 · continued past guide");
  });
});

describe("botControlLabel", () => {
  it("does not expose configured provider or model identities", () => {
    const label = botControlLabel({
      source: "external-model",
      modelName: "openai-npc-actions",
      modelVersion: "gpt-5.6"
    });

    expect(label).toBe("AI policy active");
    expect(label).not.toMatch(/gpt|openai/i);
  });
});
