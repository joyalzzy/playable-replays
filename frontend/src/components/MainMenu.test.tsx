import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MomentSummary, TelemetryMatch } from "../types";
import { MainMenu } from "./MainMenu";
import { TelemetryHub } from "./TelemetryHub";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const tutorialMoment: MomentSummary = {
  id: "objective-steal-742",
  slug: "river-pit-steal",
  title: "The River Pit Steal",
  description: "Secure the objective or escape.",
  map: "Synthetic Rift",
  category: "objective-contest",
  skillLevel: "advanced",
  reasonTags: ["objective-steal"],
  highlightScore: 0.83
};

describe("MainMenu", () => {
  it("routes the three menu icons and lists fixed tutorial scenarios", () => {
    const openTutorial = vi.fn();
    const openTelemetry = vi.fn();
    const openGuide = vi.fn();
    const openScenario = vi.fn();

    render(
      <MainMenu
        moments={[tutorialMoment]}
        onOpenTutorial={openTutorial}
        onOpenTelemetry={openTelemetry}
        onOpenGuide={openGuide}
        onOpenScenario={openScenario}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Tutorial" }));
    fireEvent.click(screen.getByRole("button", { name: "Open Match Telemetry" }));
    fireEvent.click(screen.getByRole("button", { name: "Open Beginner Guide" }));

    expect(openTutorial).toHaveBeenCalledOnce();
    expect(openTelemetry).toHaveBeenCalledOnce();
    expect(openGuide).toHaveBeenCalledOnce();
    expect(screen.getByRole("heading", { name: "Fixed scenario library" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /The River Pit Steal/i }));
    expect(openScenario).toHaveBeenCalledWith(tutorialMoment);
  });
});

describe("TelemetryHub", () => {
  it("describes the guarded detector-to-preview workflow", () => {
    const openTutorial = vi.fn();
    render(<TelemetryHub onBack={vi.fn()} onOpenTutorial={openTutorial} />);

    expect(screen.getByRole("heading", { name: "Match Telemetry" })).toBeInTheDocument();
    expect(screen.getByText("Detect")).toBeInTheDocument();
    expect(screen.getByText("Draft")).toBeInTheDocument();
    expect(screen.getByText("Author")).toBeInTheDocument();
    expect(screen.getByText("Validate and preview")).toBeInTheDocument();
    expect(screen.getByText(/not automatic publishing/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Practice a fixed scenario" }));
    expect(openTutorial).toHaveBeenCalledOnce();
  });

  it("shows a final live candidate and exposes the incomplete analyst gate", async () => {
    const match: TelemetryMatch = {
      id: "telemetry-match-1",
      source: "synthetic",
      status: "finalized",
      frameCount: 13,
      lastSecond: 12,
      expectedSequence: 13,
      savedLocally: true,
      timelineAvailable: true,
      candidates: [{
        id: "candidate-0-12",
        status: "final",
        category: "team-fight-engagement",
        draftStatus: "not-created",
        detection: {
          schemaVersion: "1.0",
          startSecond: 0,
          endSecond: 12,
          score: 0.72,
          reasonTags: ["team-fight-reversal"],
          signals: { winProbabilitySwing: 0.65, eventDensity: 1, entityProximity: 0.02, resourceAsymmetry: 0.1 },
          semanticEvidence: { oneVersusManyUnitIds: [], successfulEscapeUnitIds: [], teamFightReversalSecond: 6 }
        }
      }]
    };
    const timeline = {
      matchId: match.id,
      sourceFrameCount: 3,
      sampleEvery: 1,
      truncated: false,
      frames: [
        { second: 0, units: [{ trackId: "A1", side: "a", position: { x: 45, y: 50 }, alive: true }, { trackId: "B1", side: "b", position: { x: 55, y: 50 }, alive: true }] },
        { second: 6, units: [{ trackId: "A1", side: "a", position: { x: 48, y: 52 }, alive: true }, { trackId: "B1", side: "b", position: { x: 52, y: 48 }, alive: true }] },
        { second: 12, units: [{ trackId: "A1", side: "a", position: { x: 55, y: 55 }, alive: true }, { trackId: "B1", side: "b", position: { x: 45, y: 45 }, alive: false }] }
      ],
      events: [
        { second: 0, type: "damage", count: 2 },
        { second: 6, type: "kill", count: 1 },
        { second: 12, type: "objective", count: 1 }
      ]
    } as const;
    vi.stubGlobal("EventSource", class {
      addEventListener() {}
      close() {}
    });
    const detection = match.candidates[0]!.detection;
    const scenario = {
      id: "team-fight-engagement-0-12",
      slug: "team-fight-engagement-0-12",
      title: "",
      description: "",
      map: "",
      startTimeSeconds: 0,
      seed: 1,
      maxTurns: 0,
      controlledUnitId: "",
      reasonTags: detection.reasonTags,
      signals: detection.signals,
      sourceDetection: detection,
      units: [],
      rules: {
        initialAdvantage: 0,
        victory: { kind: "", description: "", defeatDescription: "", allowEscape: false, safeZone: { x: 0, y: 0 }, safeRadius: 0, escapeTurns: 0 },
        terrain: [], referencePlan: [], referenceReasons: [],
        referenceContinuations: { move: [], hold: [], contest: [], retreat: [] }, actionDefaults: {}
      },
      authoring: {
        category: "team-fight-engagement",
        skillLevel: "",
        analystRationale: "",
        intendedTradeoffs: [],
        plausibleAlternatives: [],
        acceptanceTests: []
      }
    };
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request) => {
      const path = String(input);
      const payload = path.endsWith("/local-storage")
        ? { mode: "local-summary-only", retentionDays: 7, matchSummaryCount: 1, draftCount: 0 }
        : path.endsWith("/timeline")
        ? timeline
        : path.endsWith("/draft")
        ? {
          candidateId: "candidate-0-12", status: "incomplete",
          completionIssues: ["analyst rationale is incomplete", "acceptance tests are incomplete"],
          fieldIssues: [
            { field: "rationale", message: "analyst rationale is incomplete" },
            { field: "acceptanceTests", message: "acceptance tests are incomplete" }
          ],
          acceptanceResults: [], canPreview: false, canExport: false,
          bundle: { version: "2.1", drafts: [{ status: "draft", scenario }] }
        }
        : path.endsWith("/matches") ? { matches: [match] } : match;
      return { ok: true, status: path.endsWith("/draft") ? 201 : 200, json: async () => payload } as Response;
    }));

    render(<TelemetryHub onBack={vi.fn()} onOpenTutorial={vi.fn()} />);

    expect(await screen.findByText("Team Fight Engagement")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "What happened on the map" })).toBeInTheDocument();
    expect(screen.getByText("Reversal at 6s")).toBeInTheDocument();
    expect(screen.queryByText("blue-carry")).not.toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Summary-only storage" })).toBeInTheDocument();
    expect(screen.getByLabelText("Local data retention")).toHaveValue("7");
    fireEvent.click(screen.getByRole("button", { name: "Jump to detected moment" }));
    expect(screen.getByRole("slider", { name: "Telemetry time" })).toHaveValue("6");
    fireEvent.click(screen.getByRole("button", { name: "Create guarded draft" }));
    expect(await screen.findByRole("heading", { name: "Turn the detected highlight into a playable lesson" })).toBeInTheDocument();
    expect(screen.getByText("Publication locked")).toBeInTheDocument();
    expect(screen.getAllByText("analyst rationale is incomplete").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Preview locally" })).toBeDisabled();
  });
});
