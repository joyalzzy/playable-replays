export type Point = { x: number; y: number };

export type UnitClass =
  | "tank"
  | "fighter"
  | "marksman"
  | "mage"
  | "support"
  | "assassin";

export type Unit = {
  id: string;
  team: "blue" | "red";
  role: string;
  class: UnitClass;
  position: Point;
  hp: number;
  maxHp: number;
  moveRange: number;
  attackRange: number;
  cooldownTurns: number;
  visible: boolean;
  alive: boolean;
};

export type ActionType =
  | "move"
  | "hold"
  | "contest"
  | "retreat"
  | "dodge"
  | "outplay";

export type Action = {
  type: ActionType;
  target?: Point;
};

export type LogEntry = {
  turn: number;
  actor: "user" | "policy";
  action: string;
  message: string;
};

export type Session = {
  id: string;
  momentId: string;
  controlledUnitId: string;
  turn: number;
  maxTurns: number;
  status: "active" | "won" | "lost";
  score: number;
  winProbability: number;
  referenceAction: Action;
  legalActions: ActionType[];
  units: Unit[];
  log: LogEntry[];
};

export type MomentSummary = {
  id: string;
  slug: string;
  title: string;
  description: string;
  map: string;
  reasonTags: string[];
  highlightScore: number;
};

export type ApiError = { error: { code: string; message: string } };
