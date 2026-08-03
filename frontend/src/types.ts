export type Point = { x: number; y: number };

export type Unit = {
  id: string;
  team: "blue" | "red";
  role: string;
  policy: string;
  position: Point;
  hp: number;
  maxHp: number;
  attackRange: number;
  attackDamage: number;
  moveSpeed: number;
  armor: number;
  visionRange: number;
  attackCooldown: number;
  cooldownTurns: number;
  shield: number;
  guarded: boolean;
  visible: boolean;
  alive: boolean;
};

export type TerrainFeature = {
  id: string;
  label: string;
  kind: string;
  position: Point;
  radius: number;
  moveMultiplier: number;
  blocksVision: boolean;
};

export type ObjectiveState = {
  id: string;
  label: string;
  position: Point;
  radius: number;
  blueProgress: number;
  redProgress: number;
  requiredProgress: number;
  status: string;
};

export type ActionType = "move" | "hold" | "contest" | "retreat";

export type Action = {
  type: ActionType;
  target?: Point;
};

export type LogEntry = {
  turn: number;
  actor: "user" | "ally" | "enemy" | "system";
  kind: string;
  action: string;
  actorId?: string;
  targetId?: string;
  value?: number;
  message: string;
};

export type ReferenceOutcome = {
  firstAction: Action;
  status: "won" | "lost";
  turns: number;
  advantage: number;
  outcomeReason: string;
  keyEvents: string[];
};

export type BestCaseAlternative = {
  action: Action;
  status: "won" | "lost";
  turns: number;
  advantage: number;
  outcomeReason: string;
};

export type BestCaseStep = {
  turn: number;
  action: Action;
  reason: string;
  advantageBefore: number;
  advantageAfter: number;
  keyEvents: string[];
  alternatives: BestCaseAlternative[];
};

export type BestCaseLine = {
  status: "won" | "lost";
  turns: number;
  advantage: number;
  outcomeReason: string;
  method: string;
  steps: BestCaseStep[];
};

export type Session = {
  id: string;
  momentId: string;
  controlledUnitId: string;
  scenarioGoal: string;
  turn: number;
  maxTurns: number;
  status: "active" | "won" | "lost";
  outcomeReason?: string;
  advantage: number;
  escapeProgress: number;
  escapeTurnsRequired: number;
  visibleEnemyCount: number;
  unknownEnemyCount: number;
  visionLimited: boolean;
  objective?: ObjectiveState;
  terrain: TerrainFeature[];
  lastReferenceAction?: Action;
  referenceReason?: string;
  referenceOutcomes?: ReferenceOutcome[];
  bestCase?: BestCaseLine;
  legalActions: ActionType[];
  units: Unit[];
  log: LogEntry[];
  debrief?: string[];
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
