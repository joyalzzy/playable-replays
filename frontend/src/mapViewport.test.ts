import { describe, expect, it } from "vitest";
import { mapViewportForMoment } from "./mapViewport";

describe("mapViewportForMoment", () => {
  const momentIds = [
    "objective-steal-742",
    "teamfight-reversal-1091",
    "objective-contest-318",
    "teamfight-engagement-864",
    "escape-412",
    "escape-1260",
    "positioning-552",
    "positioning-931",
    "resource-trade-678",
    "resource-trade-1188",
    "vision-uncertainty-356",
    "vision-uncertainty-1004"
  ];

  it("defines a focused, undistorted viewport for every authored scenario", () => {
    for (const momentId of momentIds) {
      const viewport = mapViewportForMoment(momentId);
      expect(viewport, momentId).toBeDefined();
      expect(viewport?.xMin, momentId).toBeGreaterThanOrEqual(0);
      expect(viewport?.yMin, momentId).toBeGreaterThanOrEqual(0);
      expect(viewport?.xMax, momentId).toBeLessThanOrEqual(100);
      expect(viewport?.yMax, momentId).toBeLessThanOrEqual(100);
      expect(viewport && viewport.xMax - viewport.xMin, momentId).toBeLessThan(100);
      expect(viewport && viewport.xMax - viewport.xMin, momentId).toBe(viewport && viewport.yMax - viewport.yMin);
      expect(viewport?.label, momentId).toMatch(/^Focused view · /);
    }
  });

  it("leaves unknown scenarios on the full map", () => {
    expect(mapViewportForMoment("unknown-scenario")).toBeUndefined();
  });
});
