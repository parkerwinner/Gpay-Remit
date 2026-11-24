import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import RecurringSchedules from "./RecurringSchedules";
import * as api from "../services/api";

jest.mock("../services/api", () => ({
  getRecurringSchedules: jest.fn(),
  createRecurringSchedule: jest.fn(),
  pauseRecurringSchedule: jest.fn(),
  resumeRecurringSchedule: jest.fn(),
  cancelRecurringSchedule: jest.fn(),
}));

const SENDER = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF";
const RECIPIENT = "GCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC";

describe("RecurringSchedules", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("creates, pauses, resumes, and cancels recurring schedules", async () => {
    const activeSched = {
      id: 1,
      recipient_account: RECIPIENT,
      amount: 100000000,
      currency: "USDC",
      frequency: "monthly",
      status: "active",
      next_run_at: "2026-06-01T00:00:00Z",
    };
    const pausedSched = { ...activeSched, id: 2, status: "paused" };

    api.getRecurringSchedules.mockResolvedValue({ data: { data: [activeSched, pausedSched] } });
    api.createRecurringSchedule.mockResolvedValue({ data: activeSched });
    api.pauseRecurringSchedule.mockResolvedValue({ data: { ...activeSched, status: "paused" } });
    api.resumeRecurringSchedule.mockResolvedValue({ data: { ...pausedSched, status: "active" } });
    api.cancelRecurringSchedule.mockResolvedValue({ data: { ...activeSched, status: "cancelled" } });

    render(<RecurringSchedules />);

    expect(await screen.findByText(/10.00 USDC \(monthly\)/i)).toBeInTheDocument();

    // Create flow with confirmation
    fireEvent.change(screen.getByLabelText("Sender Stellar Address"), { target: { value: SENDER } });
    fireEvent.change(screen.getByLabelText("Recipient Stellar Address"), { target: { value: RECIPIENT } });
    fireEvent.change(screen.getByLabelText("Amount"), { target: { value: "25" } });
    fireEvent.click(screen.getByText("Review Schedule"));
    expect(await screen.findByText("Confirm Recurring Schedule")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Confirm Schedule"));
    await waitFor(() => expect(api.createRecurringSchedule).toHaveBeenCalled());

    // Pause, Resume, Cancel
    fireEvent.click(screen.getByText("Pause"));
    await waitFor(() => expect(api.pauseRecurringSchedule).toHaveBeenCalledWith(1));

    fireEvent.click(screen.getByText("Resume"));
    await waitFor(() => expect(api.resumeRecurringSchedule).toHaveBeenCalledWith(2));

    fireEvent.click(screen.getAllByText("Cancel")[0]);
    await waitFor(() => expect(api.cancelRecurringSchedule).toHaveBeenCalled());
  });
});
