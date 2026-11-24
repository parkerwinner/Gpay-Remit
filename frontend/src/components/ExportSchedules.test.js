import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ExportSchedules from "./ExportSchedules";
import * as api from "../services/api";

jest.mock("../services/api", () => ({
  getExportSchedules: jest.fn(),
  createExportSchedule: jest.fn(),
  updateExportSchedule: jest.fn(),
  deleteExportSchedule: jest.fn(),
}));

describe("ExportSchedules", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test("renders existing export schedules and creates a new one", async () => {
    api.getExportSchedules.mockResolvedValue({
      data: [{ id: 1, frequency: "daily", format: "csv", destination: "email", is_active: true }],
    });
    api.createExportSchedule.mockResolvedValue({
      data: { id: 2, frequency: "weekly", format: "csv", destination: "email", is_active: true },
    });

    render(<ExportSchedules />);

    expect(await screen.findByText(/daily/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Create Export Schedule/i }));

    await waitFor(() => {
      expect(api.createExportSchedule).toHaveBeenCalledWith({
        frequency: "weekly",
        format: "csv",
        destination: "email",
      });
    });
  });
});
