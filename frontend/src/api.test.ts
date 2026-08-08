import { afterEach, describe, expect, it, vi } from "vitest";
import { dodgeProjectile } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("dodgeProjectile", () => {
  it("uses the dedicated session dodge endpoint", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ id: "session-1" })
    } as Response));
    vi.stubGlobal("fetch", fetchMock);

    await dodgeProjectile("session-1");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/sessions/session-1/dodge",
      expect.objectContaining({ method: "POST" })
    );
  });
});
