import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import RemittanceForm from "./RemittanceForm";
import * as api from "../services/api";
import { createMockRemittance } from "../test/factories";

jest.mock("../services/api");

describe("RemittanceForm", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    api.getExchangeRate.mockResolvedValue({ data: { rate: 0.92 } });
  });

  test("renders all remittance form fields and initial state", () => {
    render(<RemittanceForm />);

    expect(screen.getByLabelText(/Sender ID/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Recipient ID/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Recipient Stellar Address/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Amount/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Send Remittance/i })).toBeInTheDocument();
  });

  test("validates required fields on submission and shows error messages", async () => {
    render(<RemittanceForm />);

    fireEvent.click(screen.getByRole("button", { name: /Send Remittance/i }));

    await waitFor(() => {
      expect(screen.getByText(/Sender ID is required/i)).toBeInTheDocument();
    });
  });

  test("submits form with valid data and displays success confirmation", async () => {
    const mockRemittance = createMockRemittance({
      id: "rem_12345",
      amount: "100.00",
      status: "completed",
    });
    api.sendRemittance.mockResolvedValueOnce({ data: mockRemittance });

    render(<RemittanceForm />);

    // Valid 56-character Stellar public key starting with G
    const validStellarAddress = "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H";

    fireEvent.change(screen.getByLabelText(/Sender ID/i), { target: { value: "user_sender_1" } });
    fireEvent.change(screen.getByLabelText(/Recipient ID/i), { target: { value: "user_recipient_2" } });
    fireEvent.change(screen.getByLabelText(/Recipient Stellar Address/i), { target: { value: validStellarAddress } });
    fireEvent.change(screen.getByLabelText(/Amount/i), { target: { value: "100.00" } });

    fireEvent.click(screen.getByRole("button", { name: /Send Remittance/i }));

    await waitFor(() => {
      expect(api.sendRemittance).toHaveBeenCalledTimes(1);
      expect(screen.getByText(/Remittance Sent Successfully/i)).toBeInTheDocument();
    });
  });

  test("displays error message when API call fails", async () => {
    api.sendRemittance.mockRejectedValueOnce({
      response: { data: { error: "Insufficient balance in escrow" } },
    });

    render(<RemittanceForm />);

    const validStellarAddress = "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H";

    fireEvent.change(screen.getByLabelText(/Sender ID/i), { target: { value: "user_sender_1" } });
    fireEvent.change(screen.getByLabelText(/Recipient ID/i), { target: { value: "user_recipient_2" } });
    fireEvent.change(screen.getByLabelText(/Recipient Stellar Address/i), { target: { value: validStellarAddress } });
    fireEvent.change(screen.getByLabelText(/Amount/i), { target: { value: "100.00" } });

    fireEvent.click(screen.getByRole("button", { name: /Send Remittance/i }));

    await waitFor(() => {
      expect(screen.getByText(/Insufficient balance in escrow/i)).toBeInTheDocument();
    });
  });
});
