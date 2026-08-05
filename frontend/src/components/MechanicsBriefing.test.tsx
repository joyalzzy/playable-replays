import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MechanicsBriefing } from "./MechanicsBriefing";

afterEach(cleanup);

const briefing = {
  mechanics: [
    {
      elementId: "outer-shrine",
      name: "Outer Shrine",
      description: "Uncontested presence builds control.",
      roleInScenario: "Blue must clear the defender before capturing it."
    },
    {
      elementId: "shrine-ring",
      name: "Shrine Ring",
      description: "The approach area slows movement slightly.",
      roleInScenario: "Only the inner objective ring awards progress."
    }
  ]
};

describe("MechanicsBriefing", () => {
  it("explains what each mechanic does and its scenario role", () => {
    render(
      <MechanicsBriefing
        briefing={briefing}
        understood={false}
        disabled={false}
        onUnderstand={vi.fn()}
      />
    );

    expect(screen.getByRole("heading", { name: "Outer Shrine" })).toBeInTheDocument();
    expect(screen.getByText("Uncontested presence builds control.")).toBeInTheDocument();
    expect(screen.getByText("Blue must clear the defender before capturing it.")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Input locked");
  });

  it("uses an explicit acknowledgement to unlock commands", () => {
    const onUnderstand = vi.fn();
    render(
      <MechanicsBriefing
        briefing={briefing}
        understood={false}
        disabled={false}
        onUnderstand={onUnderstand}
      />
    );

    fireEvent.click(screen.getByRole("checkbox", { name: /i understand these scenario mechanics/i }));
    expect(onUnderstand).toHaveBeenCalledWith(true);
  });
});
