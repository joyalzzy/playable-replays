import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocalDataControls } from "./LocalDataControls";

afterEach(cleanup);

describe("LocalDataControls", () => {
  it("changes retention and requires explicit deletion confirmation", async () => {
    const changeRetention = vi.fn(async () => undefined);
    const deleteSelected = vi.fn(async () => undefined);
    const deleteAll = vi.fn(async () => undefined);
    render(
      <LocalDataControls
        status={{ mode: "local-summary-only", retentionDays: 7, matchSummaryCount: 2, draftCount: 1 }}
        selectedMatchId="telemetry-match-2"
        busy={false}
        onRetentionChange={changeRetention}
        onDeleteSelected={deleteSelected}
        onDeleteAll={deleteAll}
      />
    );

    fireEvent.change(screen.getByLabelText("Local data retention"), { target: { value: "30" } });
    expect(changeRetention).toHaveBeenCalledWith(30);

    fireEvent.click(screen.getByRole("button", { name: "Delete selected replay data" }));
    expect(screen.getByRole("alert")).toHaveTextContent("telemetry-match-2");
    expect(deleteSelected).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Confirm deletion" }));
    await waitFor(() => expect(deleteSelected).toHaveBeenCalledOnce());

    fireEvent.click(screen.getByRole("button", { name: "Delete all local telemetry" }));
    expect(screen.getByRole("alert")).toHaveTextContent("every local telemetry summary");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(deleteAll).not.toHaveBeenCalled();
  });
});
