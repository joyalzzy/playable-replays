import type { MomentSummary } from "./types";

const difficultyRank: Record<MomentSummary["skillLevel"], number> = {
  beginner: 0,
  intermediate: 1,
  advanced: 2
};

export function sortMomentsByDifficulty(moments: MomentSummary[]) {
  return [...moments].sort(
    (left, right) => difficultyRank[left.skillLevel] - difficultyRank[right.skillLevel]
  );
}

export function scenarioOptionLabel(moment: MomentSummary) {
  const difficulty = moment.skillLevel.charAt(0).toUpperCase() + moment.skillLevel.slice(1);
  return `(${difficulty}) ${moment.title}`;
}
