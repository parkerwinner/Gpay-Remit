import React, { useState } from "react";
import { stepUpMFA } from "../services/api";

function StepUp2FAModal({ isOpen, threshold, onVerified, onCancel }) {
  const [code, setCode] = useState("");
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const res = await stepUpMFA(code);
      const token = res?.data?.step_up_token;
      if (!token) {
        setError("Verification failed: missing step-up token.");
        return;
      }
      setCode("");
      onVerified(token);
    } catch (err) {
      setError(err?.message || err?.response?.data?.error || "Invalid 2FA code.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      className="step-up-2fa-modal-backdrop"
      role="dialog"
      aria-modal="true"
      aria-labelledby="step-up-2fa-title"
    >
      <div className="step-up-2fa-modal">
        <h3 id="step-up-2fa-title">Two-Factor Verification Required</h3>
        <p>
          This transfer exceeds the security threshold
          {threshold ? ` (${(threshold / 100).toFixed(2)})` : ""}. Enter your 6-digit
          authenticator code to continue.
        </p>

        {error && (
          <div role="alert" className="error" aria-live="assertive">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <label htmlFor="step-up-totp-code">6-digit 2FA code</label>
          <input
            id="step-up-totp-code"
            type="text"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="[0-9]{6}"
            maxLength={6}
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
            required
            disabled={loading}
            placeholder="000000"
          />
          <div className="modal-actions">
            <button type="submit" disabled={loading || code.length !== 6}>
              {loading ? "Verifying…" : "Verify & Continue"}
            </button>
            <button type="button" onClick={onCancel} disabled={loading}>
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default StepUp2FAModal;
