import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import WalletDashboard from "./WalletDashboard";
import * as api from "../services/api";

jest.mock("../services/api", () => ({
  getWalletBalances: jest.fn(),
  updateWalletPreferences: jest.fn(),
}));

describe("WalletDashboard", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders balances, converted estimate, missing rate note, and preference change", async () => {
    api.getWalletBalances.mockResolvedValue({
      data: {
        balances: [
          { asset_code: "USD", balance: "100.00", estimated_value: "100.00", rate_available: true },
          { asset_code: "XLM", balance: "50.0000000", rate_available: false },
        ],
        preferred_currency: "USD",
        supported_currencies: ["USD", "EUR", "XLM"],
        estimated_total: "100.00",
        has_unconverted_balances: true,
        rate_timestamp: "2026-05-01T12:00:00Z",
      },
    });
    api.updateWalletPreferences.mockResolvedValue({ data: { preferred_currency: "EUR" } });

    render(<WalletDashboard />);

    expect(await screen.findByText(/Estimated Total \(USD\)/i)).toBeInTheDocument();
    expect(screen.getByText("Rate unavailable")).toBeInTheDocument();
    expect(screen.getByText(/excluded because exchange rates were unavailable/i)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Preferred Currency"), { target: { value: "EUR" } });
    await waitFor(() => expect(api.updateWalletPreferences).toHaveBeenCalledWith("EUR"));
  });

  it("renders empty and error states", async () => {
    api.getWalletBalances.mockRejectedValueOnce({ message: "Network failure" });
    render(<WalletDashboard />);
    expect(await screen.findByText("Network failure")).toBeInTheDocument();

    api.getWalletBalances.mockResolvedValueOnce({
      data: { balances: [], preferred_currency: "USD", supported_currencies: ["USD"] },
    });
    fireEvent.click(screen.getByText("Retry"));
    expect(await screen.findByText(/No balances found/i)).toBeInTheDocument();
  });
});
