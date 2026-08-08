import { describe, expect, it } from "vitest";
import { turnLabel } from "./format";

describe("turnLabel", () => {
  it("labels turns inside the authored guidance horizon", () => {
    expect(turnLabel(3, 5)).toBe("3/5 guided");
  });

  it("keeps live turns understandable beyond that horizon", () => {
    expect(turnLabel(6, 5)).toBe("6 · continued past guide");
  });
});
