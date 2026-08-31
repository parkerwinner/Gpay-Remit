import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import Onboarding from "./Onboarding";

describe("Onboarding Component", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test("does not render when isOpen is false", () => {
    render(<Onboarding isOpen={false} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  test("renders initial welcome step when isOpen is true", () => {
    render(<Onboarding isOpen={true} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Welcome to Gpay-Remit")).toBeInTheDocument();
    expect(screen.getByText("Getting Started")).toBeInTheDocument();
    expect(screen.getByText("Skip Tour")).toBeInTheDocument();
    expect(screen.getByText("Next →")).toBeInTheDocument();
  });

  test("navigates forward through all onboarding steps", () => {
    render(<Onboarding isOpen={true} />);

    // Step 1 -> Step 2
    fireEvent.click(screen.getByText("Next →"));
    expect(screen.getByText("Fast Cross-Border Remittances")).toBeInTheDocument();
    expect(screen.getByText("Back")).toBeInTheDocument();

    // Step 2 -> Step 3
    fireEvent.click(screen.getByText("Next →"));
    expect(screen.getByText("Escrow Invoices & Requests")).toBeInTheDocument();

    // Step 3 -> Step 4
    fireEvent.click(screen.getByText("Next →"));
    expect(screen.getByText("Live History & Enterprise SLA")).toBeInTheDocument();

    // Step 4 -> Step 5 (Last Step)
    fireEvent.click(screen.getByText("Next →"));
    expect(screen.getByText("Security & Account Settings")).toBeInTheDocument();
    expect(screen.getByText("Get Started")).toBeInTheDocument();

    // Step 5 -> Step 4 (Back button)
    fireEvent.click(screen.getByText("Back"));
    expect(screen.getByText("Live History & Enterprise SLA")).toBeInTheDocument();
  });

  test("clicking progress dot jumps to specific step", () => {
    render(<Onboarding isOpen={true} />);

    const step3Dot = screen.getByLabelText("Go to step 3");
    fireEvent.click(step3Dot);
    expect(screen.getByText("Escrow Invoices & Requests")).toBeInTheDocument();
  });

  test("closing tour sets localStorage and triggers onClose callback", () => {
    const handleClose = jest.fn();
    render(<Onboarding isOpen={true} onClose={handleClose} />);

    fireEvent.click(screen.getByLabelText("Close onboarding tour"));
    expect(handleClose).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem("gpay_onboarding_completed")).toBe("true");
  });

  test("skip button sets localStorage and triggers onClose", () => {
    const handleClose = jest.fn();
    render(<Onboarding isOpen={true} onClose={handleClose} />);

    fireEvent.click(screen.getByText("Skip Tour"));
    expect(handleClose).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem("gpay_onboarding_completed")).toBe("true");
  });

  test("completing the last step triggers onComplete and onClose", () => {
    const handleComplete = jest.fn();
    const handleClose = jest.fn();
    render(<Onboarding isOpen={true} onComplete={handleComplete} onClose={handleClose} />);

    // Jump to step 5
    fireEvent.click(screen.getByLabelText("Go to step 5"));
    expect(screen.getByText("Get Started")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Get Started"));
    expect(handleComplete).toHaveBeenCalledTimes(1);
    expect(handleClose).toHaveBeenCalledTimes(1);
    expect(localStorage.getItem("gpay_onboarding_completed")).toBe("true");
  });

  test("supports keyboard navigation with Escape, ArrowRight, and ArrowLeft", () => {
    const handleClose = jest.fn();
    render(<Onboarding isOpen={true} onClose={handleClose} />);

    // Arrow right advances
    fireEvent.keyDown(window, { key: "ArrowRight" });
    expect(screen.getByText("Fast Cross-Border Remittances")).toBeInTheDocument();

    // Arrow left goes back
    fireEvent.keyDown(window, { key: "ArrowLeft" });
    expect(screen.getByText("Welcome to Gpay-Remit")).toBeInTheDocument();

    // Escape closes
    fireEvent.keyDown(window, { key: "Escape" });
    expect(handleClose).toHaveBeenCalledTimes(1);
  });
});
