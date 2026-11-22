import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import InvoiceViewer from "./InvoiceViewer";
import * as api from "../services/api";
import { createMockInvoice } from "../test/factories";

jest.mock("../services/api");

describe("InvoiceViewer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test("renders loading state initially", () => {
    api.getInvoices.mockReturnValue(new Promise(() => {})); // pending promise
    render(<InvoiceViewer />);

    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  test("renders list of invoices successfully", async () => {
    const mockInvoices = [
      createMockInvoice({ id: "inv_1", invoice_number: "INV-001", amount: 15000, currency: "USD" }),
      createMockInvoice({ id: "inv_2", invoice_number: "INV-002", amount: 30000, currency: "EUR" }),
    ];
    api.getInvoices.mockResolvedValueOnce({ data: mockInvoices });

    render(<InvoiceViewer />);

    await waitFor(() => {
      expect(screen.getByText("INV-001")).toBeInTheDocument();
      expect(screen.getByText("INV-002")).toBeInTheDocument();
    });
  });

  test("renders empty state message when no invoices exist", async () => {
    api.getInvoices.mockResolvedValueOnce({ data: [] });

    render(<InvoiceViewer />);

    await waitFor(() => {
      expect(screen.getByText(/No invoices found/i)).toBeInTheDocument();
    });
  });

  test("displays error alert when fetching invoices fails", async () => {
    api.getInvoices.mockRejectedValueOnce({
      response: { data: { error: "Network timeout fetching invoices" } },
    });

    render(<InvoiceViewer />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText(/Network timeout fetching invoices/i)).toBeInTheDocument();
    });
  });
});
