import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import PaymentTemplates from "./PaymentTemplates";
import * as api from "../services/api";

jest.mock("../services/api", () => ({
  getTemplates: jest.fn(),
  createTemplate: jest.fn(),
  updateTemplate: jest.fn(),
  deleteTemplate: jest.fn(),
  sendFromTemplate: jest.fn(),
  calculateFee: jest.fn(),
}));

const VALID_ADDR = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF";

describe("PaymentTemplates", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists templates and supports create, edit, delete, and send confirmation", async () => {
    const sample = {
      id: 1,
      template_name: "Rent",
      recipient_account: VALID_ADDR,
      amount: 100,
      asset_code: "USDC",
    };
    api.getTemplates.mockResolvedValue({ data: { data: [sample] } });
    api.createTemplate.mockResolvedValue({ data: { id: 2, ...sample, template_name: "Groceries" } });
    api.updateTemplate.mockResolvedValue({ data: { ...sample, template_name: "Updated Rent" } });
    api.deleteTemplate.mockResolvedValue({ data: { message: "deleted" } });
    api.calculateFee.mockResolvedValue({ data: { total_fee: 5000000 } });
    api.sendFromTemplate.mockResolvedValue({ data: { remittance_id: 42, status: "pending" } });

    render(<PaymentTemplates />);

    expect(await screen.findByText("Rent")).toBeInTheDocument();

    // Create
    fireEvent.change(screen.getByLabelText("Template Name"), { target: { value: "Groceries" } });
    fireEvent.change(screen.getByLabelText("Recipient Stellar Address"), { target: { value: VALID_ADDR } });
    fireEvent.change(screen.getByLabelText("Amount"), { target: { value: "50" } });
    fireEvent.click(screen.getByText("Save Template"));
    await waitFor(() => expect(api.createTemplate).toHaveBeenCalled());

    // Edit
    fireEvent.click(screen.getByText("Edit"));
    fireEvent.change(screen.getByLabelText("Template Name"), { target: { value: "Updated Rent" } });
    fireEvent.click(screen.getByText("Update Template"));
    await waitFor(() => expect(api.updateTemplate).toHaveBeenCalledWith(1, expect.objectContaining({ template_name: "Updated Rent" })));

    // Send confirmation
    fireEvent.click(screen.getByText("Send"));
    expect(await screen.findByText("Confirm Send")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Confirm & Send"));
    await waitFor(() => expect(api.sendFromTemplate).toHaveBeenCalledWith(1));
    expect(await screen.findByText(/Remittance #42 created/i)).toBeInTheDocument();

    // Delete confirmation
    fireEvent.click(screen.getByText("Delete"));
    fireEvent.click(screen.getByText("Confirm Delete"));
    await waitFor(() => expect(api.deleteTemplate).toHaveBeenCalledWith(1));
  });
});
