import type {
  Action,
  ApiError,
  MomentSummary,
  Session
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

export function takeTurn(sessionId: string, action: Action, targetUnitId?: string): Promise<Session> {
  return request<Session>(`/api/v1/sessions/${sessionId}/turns`, {
    method: "POST",
    body: JSON.stringify({ action, ...(targetUnitId ? { targetUnitId } : {}) })
  });
}

export function fireProjectile(
  sessionId: string,
  sourceUnitId: string,
  targetUnitId: string
): Promise<Session> {
  return request<Session>(`/api/v1/sessions/${sessionId}/fire`, {
    method: "POST",
    body: JSON.stringify({ sourceUnitId, targetUnitId })
  });
}

export function dodgeProjectile(sessionId: string): Promise<Session> {
  return request<Session>(`/api/v1/sessions/${sessionId}/dodge`, {
    method: "POST"
  });
}

export function resetSession(sessionId: string): Promise<Session> {
  return request<Session>(`/api/v1/sessions/${sessionId}/reset`, {
    method: "POST"
  });
}
