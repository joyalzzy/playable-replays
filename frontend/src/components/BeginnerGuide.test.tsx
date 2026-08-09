import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BeginnerGuide } from "./BeginnerGuide";

afterEach(cleanup);

describe("BeginnerGuide", () => {
  it("presents a dedicated full-page orientation and routes both actions", () => {
    const onBack = vi.fn();
    const onStartTutorial = vi.fn();

    render(<BeginnerGuide onBack={onBack} onStartTutorial={onStartTutorial} />);

    expect(screen.getByRole("heading", { name: /Read the field/i })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Beginner replay essentials" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Four core choices" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Four signals to watch" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Know the roles" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "How the replay works" })).not.toBeInTheDocument();
    expect(screen.getByText(/disengage faster toward the Blue base/i)).toBeInTheDocument();
    expect(screen.queryByText(/safe (?:zone|area)/i)).not.toBeInTheDocument();

    const carryIcon = screen.getByRole("button", { name: "About Carry" });
    const carryTooltipID = carryIcon.getAttribute("aria-describedby");
    expect(carryTooltipID).toBeTruthy();
    expect(document.getElementById(carryTooltipID!)).toHaveTextContent(/sustained damage/i);

    const fogIcon = screen.getByRole("button", { name: "About Fog warning" });
    const fogTooltipID = fogIcon.getAttribute("aria-describedby");
    expect(document.getElementById(fogTooltipID!)).toHaveTextContent(/enemy position is unknown/i);

    const projectileIcon = screen.getByRole("button", { name: "About Incoming projectile" });
    const projectileTooltipID = projectileIcon.getAttribute("aria-describedby");
    expect(document.getElementById(projectileTooltipID!)).toHaveTextContent(/two charges per scenario/i);

    fireEvent.click(screen.getByRole("button", { name: /Start your first tutorial/i }));
    fireEvent.click(screen.getByRole("button", { name: "Back to main menu" }));

    expect(onStartTutorial).toHaveBeenCalledOnce();
    expect(onBack).toHaveBeenCalledOnce();
  });
});
