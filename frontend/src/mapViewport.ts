export type MapViewport = {
  xMin: number;
  xMax: number;
  yMin: number;
  yMax: number;
  label: string;
};

export const fullMapViewport: MapViewport = {
  xMin: 0,
  xMax: 100,
  yMin: 0,
  yMax: 100,
  label: "Full map"
};

const scenarioViewports: Record<string, MapViewport> = {
  "objective-steal-742": {
    xMin: 13,
    xMax: 83,
    yMin: 19,
    yMax: 89,
    label: "Focused view · River pit"
  },
  "teamfight-reversal-1091": {
    xMin: 5,
    xMax: 75,
    yMin: 30,
    yMax: 100,
    label: "Focused view · Blue base gate"
  },
  "objective-contest-318": {
    xMin: 20,
    xMax: 76,
    yMin: 19,
    yMax: 75,
    label: "Focused view · Outer shrine"
  },
  "teamfight-engagement-864": {
    xMin: 20,
    xMax: 74,
    yMin: 31,
    yMax: 85,
    label: "Focused view · Lower-river flank"
  },
  "escape-412": {
    xMin: 16,
    xMax: 100,
    yMin: 10,
    yMax: 94,
    label: "Focused view · Tower lane"
  },
  "escape-1260": {
    xMin: 0,
    xMax: 74,
    yMin: 20,
    yMax: 94,
    label: "Focused view · Lower-left breakout"
  },
  "positioning-552": {
    xMin: 5,
    xMax: 72,
    yMin: 26,
    yMax: 93,
    label: "Focused view · Support-side lane"
  },
  "positioning-931": {
    xMin: 24,
    xMax: 78,
    yMin: 34,
    yMax: 88,
    label: "Focused view · Western choke"
  },
  "resource-trade-678": {
    xMin: 15,
    xMax: 81,
    yMin: 21,
    yMax: 87,
    label: "Focused view · Cannon-wave lane"
  },
  "resource-trade-1188": {
    xMin: 0,
    xMax: 76,
    yMin: 24,
    yMax: 100,
    label: "Focused view · Red buff exit"
  },
  "vision-uncertainty-356": {
    xMin: 6,
    xMax: 76,
    yMin: 27,
    yMax: 97,
    label: "Focused view · Allied lane pocket"
  },
  "vision-uncertainty-1004": {
    xMin: 0,
    xMax: 70,
    yMin: 27,
    yMax: 97,
    label: "Focused view · Support-side exit"
  }
};

export function mapViewportForMoment(momentId: string) {
  return scenarioViewports[momentId];
}
