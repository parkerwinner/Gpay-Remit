import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import TransactionHistory from "./TransactionHistory";
import * as api from "../services/api";

jest.mock("../services/api");

describe("TransactionHistory Payment Tags (#455)", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    api.getRemittances.mockResolvedValue({
      data: {
        data: [
          {
            id: 1,
            sender_id: 1,
            recipient_id: 2,
            amount: 10000,
            currency: "USD",
            status: "completed",
            created_at: "2026-01-01T00:00:00Z",
            tags: ["family", "rent"],
          },
        ],
        total_count: 1,
      },
    });
    api.getTagSummary.mockResolvedValue({
      data: {
        tags: [
          {
            tag: "family",
            payment_count: 1,
            totals: [
              { asset: "USD", total: 10000 },
              { asset: "XLM", total: 50000000 },
            ],
          },
        ],
      },
    });
    api.autocompleteTags.mockResolvedValue({
      data: {
        tags: [
          { tag: "utilities", count: 4 },
          { tag: "urgent", count: 2 },
        ],
      },
    });
    api.updateRemittanceTags.mockResolvedValue({
      data: { payment_id: 1, tags: ["family", "rent", "utilities"] },
    });
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  test("renders payment tags and per-asset tag summary", async () => {
    render(
      <MemoryRouter initialEntries={["/transactions"]}>
        <TransactionHistory />
      </MemoryRouter>
    );

    expect(await screen.findByText("Tag Summary (Completed Payments)")).toBeInTheDocument();
    expect(screen.getByText(/100\.00 USD/)).toBeInTheDocument();
    expect(screen.getByText(/5\.0000000 XLM/)).toBeInTheDocument();
  });

  test("shows error state when tag summary fails to load", async () => {
    api.getTagSummary.mockRejectedValueOnce(new Error("Summary unavailable"));
    render(
      <MemoryRouter initialEntries={["/transactions"]}>
        <TransactionHistory />
      </MemoryRouter>
    );

    expect(await screen.findByText("Summary unavailable")).toBeInTheDocument();
  });

  test("supports accessible combobox keyboard navigation, autocomplete, and filter chips", async () => {
    render(
      <MemoryRouter initialEntries={["/transactions"]}>
        <TransactionHistory />
      </MemoryRouter>
    );

    const combobox = await screen.findByRole("combobox", { name: /filter by tags/i });
    expect(combobox).toHaveAttribute("aria-expanded", "false");

    fireEvent.change(combobox, { target: { value: "u" } });
    await act(async () => {
      jest.advanceTimersByTime(260);
    });

    await waitFor(() => expect(api.autocompleteTags).toHaveBeenCalled());
    expect(combobox).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("2 tag suggestions available")).toBeInTheDocument();

    // ArrowDown highlights first option, Enter selects it
    fireEvent.keyDown(combobox, { key: "ArrowDown" });
    fireEvent.keyDown(combobox, { key: "Enter" });

    await waitFor(() => {
      expect(api.getRemittances).toHaveBeenCalledWith(1, 20, {}, ["utilities"]);
    });

    // Comma adds free-text tag, Backspace on empty removes last tag
    fireEvent.change(combobox, { target: { value: "travel" } });
    fireEvent.keyDown(combobox, { key: "," });
    await waitFor(() => {
      expect(api.getRemittances).toHaveBeenCalledWith(1, 20, {}, ["utilities", "travel"]);
    });

    fireEvent.keyDown(combobox, { key: "Backspace" });
    await waitFor(() => {
      expect(api.getRemittances).toHaveBeenCalledWith(1, 20, {}, ["utilities"]);
    });
  });
});
