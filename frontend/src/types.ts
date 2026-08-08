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
  policy: string;
  position: Point;
  hp: number;
  maxHp: number;
  moveRange: number;
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

export type ScenarioMechanic = {
  elementId: string;
  name: string;
  description: string;
  roleInScenario: string;
};

export type MechanicBriefing = {
  mechanics: ScenarioMechanic[];
};

export type ActionType = "move" | "hold" | "contest" | "retreat";

export type Action = {
  type: ActionType;
  target?: Point;
};

export type LogEntry = {
  turn: number;
  actor: "user" | "ally" | "enemy" | "policy" | "system";
  kind: string;
  action: string;
  actorId?: string;
  targetId?: string;
  value?: number;
  message: string;
};

export type Turret = {
  id: string;
  team: "blue" | "red";
  lane: "top" | "middle" | "bottom";
  position: Point;
  hp: number;
  maxHp: number;
  alive: boolean;
};

export type Projectile = {
  id: string;
  team: "blue" | "red";
  sourceUnitId: string;
  targetUnitId: string;
  position: Point;
  target: Point;
  damage: number;
};

export type BotControl = {
  source: "pending" | "external-model" | "deterministic-fallback";
  modelName?: string;
  modelVersion?: string;
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
  mechanicBriefing?: MechanicBriefing;
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
  turrets: Turret[];
  projectiles: Projectile[];
  projectileCharges: number;
  projectileAvailable: boolean;
  dodgeCharges: number;
  dodgeAvailable: boolean;
  botControl: BotControl;
  log: LogEntry[];
  debrief?: string[];
};

export type MomentSummary = {
  id: string;
  slug: string;
  title: string;
  description: string;
  map: string;
  category: "objective-contest" | "team-fight-engagement" | "escape" | "positioning" | "resource-trade" | "vision-uncertainty";
  skillLevel: "beginner" | "intermediate" | "advanced";
  reasonTags: string[];
  highlightScore: number;
};

export type ApiError = { error: { code: string; message: string } };
