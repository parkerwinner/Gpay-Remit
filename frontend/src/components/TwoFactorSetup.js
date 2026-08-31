// Two-Factor Authentication (TOTP) UI — backed by pquerna/otp on the server.
// Allows authenticated users to enable/disable 2FA and verifies via /auth/mfa/* endpoints.

import React, { useState, useEffect } from "react";
import { setupMFA, verifyMFA, disableMFA, getMFAStatus } from "../services/api";

function TwoFactorSetup() {
  const [mfaEnabled, setMfaEnabled] = useState(null);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [secret, setSecret] = useState("");
  const [qrCode, setQrCode] = useState("");
  const [step, setStep] = useState("idle"); // idle | setup | verify | done
  const [message, setMessage] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);

  const fetchStatus = async () => {
    try {
      const res = await getMFAStatus();
      setMfaEnabled(res.data.mfa_enabled);
    } catch {
      setMfaEnabled(false);
    }
  };

  useEffect(() => {
    fetchStatus();
  }, []);

  const handleSetup = async (e) => {
    e.preventDefault();
    setError(null);
    setMessage(null);
    setLoading(true);
    try {
      const res = await setupMFA(password);
      setSecret(res.data.secret);
      setQrCode(res.data.qr_code);
      setStep("verify");
      setMessage("Scan the QR code with your authenticator app and enter the 6-digit code.");
    } catch (err) {
      const msg = err?.response?.data?.error || err?.message || "Failed to setup 2FA.";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  const handleVerify = async (e) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await verifyMFA(code);
      setMessage("Two-factor authentication enabled successfully.");
      setStep("done");
      setMfaEnabled(true);
    } catch (err) {
      const msg = err?.response?.data?.error || err?.message || "Invalid code.";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  const handleDisable = async (e) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      // DisableMFA endpoint expects password in LoginRequest shape
      await disableMFA(password);
      setMessage("Two-factor authentication disabled.");
      setMfaEnabled(false);
      setStep("idle");
      setSecret("");
      setQrCode("");
    } catch (err) {
      const msg = err?.response?.data?.error || err?.message || "Failed to disable 2FA.";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  if (mfaEnabled === null) {
    return <div aria-busy="true">Loading 2FA status…</div>;
  }

  return (
    <div className="two-factor-setup" aria-labelledby="mfa-heading">
      <h3 id="mfa-heading">Two-Factor Authentication</h3>
      <p role="status" aria-live="polite">
        Status: <strong>{mfaEnabled ? "Enabled" : "Disabled"}</strong>
      </p>

      {message && (
        <div role="status" className="success" aria-live="polite">
          {message}
        </div>
      )}
      {error && (
        <div role="alert" className="error" aria-live="assertive">
          {error}
        </div>
      )}

      {!mfaEnabled && step !== "verify" && step !== "done" && (
        <form onSubmit={handleSetup} aria-label="Setup 2FA form">
          <label htmlFor="mfa-password">Confirm password to setup 2FA</label>
          <input
            id="mfa-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            aria-required="true"
            placeholder="Current password"
          />
          <button type="submit" disabled={loading} aria-busy={loading}>
            {loading ? "Setting up…" : "Setup 2FA"}
          </button>
        </form>
      )}

      {step === "verify" && (
        <div className="mfa-verify">
          {qrCode && (
            <div className="qr-code">
              <p>Scan this QR code with Google Authenticator / Authy:</p>
              <img src={qrCode} alt="TOTP QR code" style={{ maxWidth: 200 }} />
              <p>
                <small>Manual secret: <code>{secret}</code></small>
              </p>
            </div>
          )}
          <form onSubmit={handleVerify} aria-label="Verify 2FA form">
            <label htmlFor="mfa-code">Enter 6-digit code</label>
            <input
              id="mfa-code"
              type="text"
              inputMode="numeric"
              pattern="[0-9]{6}"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              required
              aria-required="true"
              placeholder="123456"
            />
            <button type="submit" disabled={loading || code.length !== 6} aria-busy={loading}>
              {loading ? "Verifying…" : "Verify & Enable"}
            </button>
          </form>
        </div>
      )}

      {mfaEnabled && (
        <form onSubmit={handleDisable} aria-label="Disable 2FA form">
          <label htmlFor="mfa-disable-password">Password to disable 2FA</label>
          <input
            id="mfa-disable-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            aria-required="true"
            placeholder="Current password"
          />
          <button type="submit" disabled={loading} aria-busy={loading} className="danger">
            {loading ? "Disabling…" : "Disable 2FA"}
          </button>
        </form>
      )}
    </div>
  );
}

export default TwoFactorSetup;
