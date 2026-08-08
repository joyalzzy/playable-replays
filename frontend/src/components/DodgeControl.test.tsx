import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DodgeControl } from "./DodgeControl";

afterEach(cleanup);

describe("DodgeControl", () => {
  it("uses an available charge immediately for a pending projectile", () => {
    const onDodge = vi.fn();
    render(
      <DodgeControl
        charges={2}
        available
        incomingProjectiles={1}
        disabled={false}
        onDodge={onDodge}
      />
    );

    expect(screen.getByText("2 dodge charges remaining")).toBeInTheDocument();
    expect(screen.getByText(/1 incoming projectile is threatening/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Dodge projectile" }));
    expect(onDodge).toHaveBeenCalledOnce();
  });

  it("disables dodge outside the server-authorized window", () => {
    render(
      <DodgeControl
        charges={1}
        available={false}
        incomingProjectiles={0}
        disabled={false}
        onDodge={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: "Dodge projectile" })).toBeDisabled();
    expect(screen.getByText(/no projectile can be dodged/i)).toBeInTheDocument();
  });

  it("announces exhausted charges", () => {
    render(
      <DodgeControl
        charges={0}
        available
        incomingProjectiles={1}
        disabled={false}
        onDodge={vi.fn()}
      />
    );

    expect(screen.getByText("0 dodge charges remaining")).toBeInTheDocument();
    expect(screen.getByText(/both dodge charges have been used/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dodge projectile" })).toBeDisabled();
  });
});
