import { afterEach, describe, expect, it, vi } from "vitest";
import { dodgeProjectile, fireProjectile, takeTurn } from "./api";

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

describe("targeted combat requests", () => {
  it("sends the selected contest target with the tactical turn", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ id: "session-1" })
    } as Response));
    vi.stubGlobal("fetch", fetchMock);

    await takeTurn("session-1", { type: "contest" }, "red-mage");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/sessions/session-1/turns",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ action: { type: "contest" }, targetUnitId: "red-mage" })
      })
    );
  });

  it("uses the separate fire endpoint for a marksman projectile", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ id: "session-1" })
    } as Response));
    vi.stubGlobal("fetch", fetchMock);

    await fireProjectile("session-1", "blue-marksman", "red-mage");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/sessions/session-1/fire",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ sourceUnitId: "blue-marksman", targetUnitId: "red-mage" })
      })
    );
  });
});
