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

export type TelemetrySignals = {
  winProbabilitySwing: number;
  eventDensity: number;
  entityProximity: number;
  resourceAsymmetry: number;
};

export type TelemetryDetection = {
  schemaVersion: "1.0";
  startSecond: number;
  endSecond: number;
  score: number;
  reasonTags: string[];
  signals: TelemetrySignals;
  semanticEvidence: {
    oneVersusManyUnitIds: string[];
    successfulEscapeUnitIds: string[];
    teamFightReversalSecond: number | null;
  };
};

export type TelemetryCandidate = {
  id: string;
  status: "provisional" | "final";
  category: MomentSummary["category"];
  draftStatus: "not-created" | "incomplete" | "ready";
  detection: TelemetryDetection;
};

export type TelemetryMatch = {
  id: string;
  source: "synthetic" | "authorized";
  status: "capturing" | "finalized";
  frameCount: number;
  lastSecond: number;
  expectedSequence: number;
  savedLocally: boolean;
  timelineAvailable: boolean;
  candidates: TelemetryCandidate[];
};

export type LocalStorageStatus = {
  mode: "local-summary-only" | "memory-only";
  retentionDays: number;
  matchSummaryCount: number;
  draftCount: number;
};

export type DeleteLocalDataResponse = {
  deletedMatches: number;
  deletedDrafts: number;
};

export type TelemetryTimelineUnit = {
  trackId: string;
  side: "a" | "b";
  position: Point;
  alive: boolean;
};

export type TelemetryTimelineFrame = {
  second: number;
  units: TelemetryTimelineUnit[];
};

export type TelemetryTimelineEvent = {
  second: number;
  type: "damage" | "kill" | "objective" | "vision-loss";
  count: number;
};

export type TelemetryTimeline = {
  matchId: string;
  sourceFrameCount: number;
  sampleEvery: number;
  truncated: boolean;
  frames: TelemetryTimelineFrame[];
  events: TelemetryTimelineEvent[];
};

export type TelemetryDraftResult = {
  candidateId: string;
  status: "incomplete" | "ready";
  completionIssues: string[];
  fieldIssues: Array<{ field: DraftField; message: string }>;
  acceptanceResults: AcceptanceResult[];
  canPreview: boolean;
  canExport: boolean;
  bundle: DraftBundle;
};

export type DraftField =
  | "status"
  | "provenance"
  | "title"
  | "description"
  | "map"
  | "difficulty"
  | "rationale"
  | "tradeoffs"
  | "alternatives"
  | "acceptanceTests"
  | "units"
  | "terrain"
  | "rules";

export type TelemetryScenario = {
  id: string;
  slug: string;
  title: string;
  description: string;
  map: string;
  startTimeSeconds: number;
  seed: number;
  maxTurns: number;
  controlledUnitId: string;
  reasonTags: string[];
  signals: TelemetrySignals;
  sourceDetection: TelemetryDetection;
  mechanicBriefing?: MechanicBriefing;
  units: Unit[];
  rules: {
    initialAdvantage: number;
    objective?: {
      id: string;
      label: string;
      position: Point;
      radius: number;
      captureTurns: number;
    };
    victory: {
      kind: string;
      targetUnitId?: string;
      description: string;
      defeatDescription: string;
      allowEscape: boolean;
      safeZone: Point;
      safeRadius: number;
      escapeTurns: number;
    };
    terrain: TerrainFeature[];
    referencePlan: Action[];
    referenceReasons: string[];
    referenceContinuations: Record<ActionType, Action[]>;
    actionDefaults: Partial<Record<ActionType, Action>>;
  };
  authoring: {
    category: MomentSummary["category"];
    skillLevel: "" | MomentSummary["skillLevel"];
    analystRationale: string;
    intendedTradeoffs: string[];
    plausibleAlternatives: Array<{
      action: Action;
      when: string;
      tradeoff: string;
    }>;
    acceptanceTests: Array<{
      name: string;
      actions: Action[];
      expectedStatus: "won" | "lost";
      expectedTerminalTurn: number;
      expectedOutcomeContains: string;
    }>;
  };
};

export type DraftBundle = {
  version: "2.1";
  drafts: [
    { status: "draft"; scenario: TelemetryScenario },
    ...Array<{ status: "draft"; scenario: TelemetryScenario }>
  ];
};

export type AcceptanceResult = {
  momentId: string;
  testName: string;
  passed: boolean;
  detail: string;
};

export type DraftPreview = {
  moment: MomentSummary;
  session: Session;
};

export type FixtureReviewPack = {
  version: "2.1";
  moments: TelemetryScenario[];
};

export type ApiError = { error: { code: string; message: string } };
