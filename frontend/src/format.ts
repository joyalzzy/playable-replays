import type { Action } from "./types";

export function advantageLabel(value: number) {
  if (value >= 0.66) return "Blue favored";
  if (value <= 0.34) return "Blue pressured";
  return "Contested";
}

export function turnLabel(turn: number, guidedTurns: number) {
  if (turn <= guidedTurns) return `${turn}/${guidedTurns} guided`;
  return `${turn} · continued past guide`;
}

export function actionLabel(action: Action) {
  if (action.type !== "move" || !action.target) return action.type;
  return `${action.type} to ${Math.round(action.target.x)}, ${Math.round(action.target.y)}`;
}
