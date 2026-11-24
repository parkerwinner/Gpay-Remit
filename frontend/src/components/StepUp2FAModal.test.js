import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import StepUp2FAModal from "./StepUp2FAModal";
import * as api from "../services/api";

jest.mock("../services/api", () => ({
  stepUpMFA: jest.fn(),
}));

describe("StepUp2FAModal", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test("does not render when closed", () => {
    const { container } = render(
      <StepUp2FAModal isOpen={false} onVerified={jest.fn()} onCancel={jest.fn()} />
    );
    expect(container.firstChild).toBeNull();
  });

  test("submits 6-digit code and invokes onVerified with step_up_token", async () => {
    api.stepUpMFA.mockResolvedValue({
      data: { step_up_token: "step-token-123", expires_in: 300 },
    });
    const onVerified = jest.fn();

    render(
      <StepUp2FAModal
        isOpen={true}
        threshold={100000}
        onVerified={onVerified}
        onCancel={jest.fn()}
      />
    );

    const input = screen.getByLabelText(/6-digit 2FA code/i);
    fireEvent.change(input, { target: { value: "123456" } });
    fireEvent.click(screen.getByRole("button", { name: /Verify & Continue/i }));

    await waitFor(() => {
      expect(api.stepUpMFA).toHaveBeenCalledWith("123456");
      expect(onVerified).toHaveBeenCalledWith("step-token-123");
    });
  });
});
