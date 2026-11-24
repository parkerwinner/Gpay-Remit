import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import EmailPreferences from "./EmailPreferences";
import * as api from "../services/api";

jest.mock("../services/api", () => ({
  getEmailPreferences: jest.fn(),
  updateEmailPreferences: jest.fn(),
}));

describe("EmailPreferences", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test("loads preferences and keeps security alerts locked on", async () => {
    api.getEmailPreferences.mockResolvedValue({
      data: {
        transaction_confirmations: true,
        marketing: false,
        security_alerts: true,
      },
    });

    render(<EmailPreferences />);

    const txCheckbox = await screen.findByLabelText(/Transaction confirmations/i);
    const marketingCheckbox = screen.getByLabelText(/marketing emails/i);
    const securityCheckbox = screen.getByLabelText(/Security alerts/i);

    expect(txCheckbox).toBeChecked();
    expect(marketingCheckbox).not.toBeChecked();
    expect(securityCheckbox).toBeChecked();
    expect(securityCheckbox).toBeDisabled();
  });

  test("toggles and saves preferences", async () => {
    api.getEmailPreferences.mockResolvedValue({
      data: {
        transaction_confirmations: true,
        marketing: false,
        security_alerts: true,
      },
    });
    api.updateEmailPreferences.mockResolvedValue({
      data: {
        transaction_confirmations: false,
        marketing: true,
        security_alerts: true,
      },
    });

    render(<EmailPreferences />);

    const txCheckbox = await screen.findByLabelText(/Transaction confirmations/i);
    const marketingCheckbox = screen.getByLabelText(/marketing emails/i);

    fireEvent.click(txCheckbox);
    fireEvent.click(marketingCheckbox);
    fireEvent.click(screen.getByRole("button", { name: /Save Preferences/i }));

    await waitFor(() => {
      expect(api.updateEmailPreferences).toHaveBeenCalledWith({
        transaction_confirmations: false,
        marketing: true,
        security_alerts: true,
      });
    });

    expect(await screen.findByText(/Email preferences saved/i)).toBeInTheDocument();
  });
});
