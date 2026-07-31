export type Point = { x: number; y: number };

export type Unit = {
  id: string;
  team: "blue" | "red";
  role: string;
  position: Point;
  hp: number;
  maxHp: number;
  cooldownTurns: number;
  visible: boolean;
  alive: boolean;
};

export type ActionType = "move" | "hold" | "contest" | "retreat";

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

