import type {
  Action,
  ApiError,
  DraftPreview,
  DeleteLocalDataResponse,
  FixtureReviewPack,
  LocalStorageStatus,
  MomentSummary,
  Session,
  TelemetryDraftResult,
  TelemetryMatch,
  TelemetryScenario,
  TelemetryTimeline
} from "./types";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers
    }
  });
  const payload: unknown = await response.json();
  if (!response.ok) {
    const message =
      isApiError(payload) ? payload.error.message : `Request failed (${response.status})`;
    throw new Error(message);
  }
  return payload as T;
}

function isApiError(payload: unknown): payload is ApiError {
  if (typeof payload !== "object" || payload === null || !("error" in payload)) {
    return false;
  }
  const error = payload.error;
  return (
    typeof error === "object" &&
    error !== null &&
    "message" in error &&
    typeof error.message === "string"
  );
}

export async function listMoments(): Promise<MomentSummary[]> {
  const result = await request<{ moments: MomentSummary[] }>("/api/v1/moments");
  return result.moments;
}

export function createSession(momentId: string): Promise<Session> {
  return request<Session>("/api/v1/sessions", {
    method: "POST",
    body: JSON.stringify({ momentId })
  });
}

export function takeTurn(sessionId: string, action: Action): Promise<Session> {
  return request<Session>(`/api/v1/sessions/${sessionId}/turns`, {
    method: "POST",
    body: JSON.stringify({ action })
  });
}

export function resetSession(sessionId: string): Promise<Session> {
  return request<Session>(`/api/v1/sessions/${sessionId}/reset`, {
    method: "POST"
  });
}

export async function listTelemetryMatches(): Promise<TelemetryMatch[]> {
  const result = await request<{ matches: TelemetryMatch[] }>("/api/v1/telemetry/matches");
  return result.matches;
}

export function getTelemetryMatch(matchId: string): Promise<TelemetryMatch> {
  return request<TelemetryMatch>(`/api/v1/telemetry/matches/${matchId}`);
}

export function getTelemetryTimeline(matchId: string): Promise<TelemetryTimeline> {
  return request<TelemetryTimeline>(`/api/v1/telemetry/matches/${matchId}/timeline`);
}

export function getLocalStorageStatus(): Promise<LocalStorageStatus> {
  return request<LocalStorageStatus>("/api/v1/local-storage");
}

export function updateLocalStorageRetention(retentionDays: number): Promise<LocalStorageStatus> {
  return request<LocalStorageStatus>("/api/v1/local-storage/retention", {
    method: "PUT",
    body: JSON.stringify({ retentionDays })
  });
}

export function deleteTelemetryMatch(matchId: string): Promise<DeleteLocalDataResponse> {
  return request<DeleteLocalDataResponse>(`/api/v1/telemetry/matches/${matchId}`, { method: "DELETE" });
}

export function deleteAllTelemetryMatches(): Promise<DeleteLocalDataResponse> {
  return request<DeleteLocalDataResponse>("/api/v1/telemetry/matches", { method: "DELETE" });
}

export function createTelemetryDraft(matchId: string, candidateId: string): Promise<TelemetryDraftResult> {
  return request<TelemetryDraftResult>(`/api/v1/telemetry/matches/${matchId}/candidates/${candidateId}/draft`, {
    method: "POST",
    body: "{}"
  });
}

function draftPath(matchId: string, candidateId: string): string {
  return `/api/v1/telemetry/matches/${matchId}/candidates/${candidateId}/draft`;
}

export function getTelemetryDraft(matchId: string, candidateId: string): Promise<TelemetryDraftResult> {
  return request<TelemetryDraftResult>(draftPath(matchId, candidateId));
}

export function updateTelemetryDraft(
  matchId: string,
  candidateId: string,
  scenario: TelemetryScenario
): Promise<TelemetryDraftResult> {
  return request<TelemetryDraftResult>(draftPath(matchId, candidateId), {
    method: "PUT",
    body: JSON.stringify({ scenario })
  });
}

export function validateTelemetryDraft(matchId: string, candidateId: string): Promise<TelemetryDraftResult> {
  return request<TelemetryDraftResult>(`${draftPath(matchId, candidateId)}/validate`, {
    method: "POST",
    body: "{}"
  });
}

export function previewTelemetryDraft(matchId: string, candidateId: string): Promise<DraftPreview> {
  return request<DraftPreview>(`${draftPath(matchId, candidateId)}/preview`, {
    method: "POST",
    body: "{}"
  });
}

export function exportTelemetryReviewPack(matchId: string, candidateId: string): Promise<FixtureReviewPack> {
  return request<FixtureReviewPack>(`${draftPath(matchId, candidateId)}/review-pack`, {
    method: "POST",
    body: "{}"
  });
}

export function subscribeTelemetryMatch(
  matchId: string,
  onMatch: (match: TelemetryMatch) => void,
  onConnectionChange: (connected: boolean) => void
): () => void {
  const source = new EventSource(`/api/v1/telemetry/matches/${matchId}/events`);
  source.addEventListener("open", () => onConnectionChange(true));
  source.addEventListener("match", (event) => {
    try {
      onMatch(JSON.parse((event as MessageEvent<string>).data) as TelemetryMatch);
    } catch {
      onConnectionChange(false);
    }
  });
  source.addEventListener("error", () => onConnectionChange(false));
  return () => source.close();
}
