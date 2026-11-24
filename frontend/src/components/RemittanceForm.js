// #110 form validation  #116 accessibility  #117 input sanitization  #118 conversion preview
import React, { useState, useEffect, useCallback } from "react";
import { sendRemittance, getExchangeRate } from "../services/api";
import {
  validateRemittanceForm,
  validateStellarAddress,
  isValidStellarAddress,
} from "../utils/validation";
import { sanitizeInput } from "../utils/sanitize";
import ConversionPreview from "./ConversionPreview";
import LoadingSpinner from "./LoadingSpinner";
import StepUp2FAModal from "./StepUp2FAModal";

const INITIAL = {
  sender_id: "",
  recipient_id: "",
  recipient_account: "",
  amount: "",
  currency: "USD",
  target_currency: "EUR",
  notes: "",
};

function FieldError({ id, message }) {
  if (!message) return null;
  return (
    <span id={id} role="alert" className="field-error" aria-live="polite">
      {message}
    </span>
  );
}

function RemittanceForm() {
  const [formData, setFormData] = useState(INITIAL);
  const [fieldErrors, setFieldErrors] = useState({});
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [submitError, setSubmitError] = useState(null);
  const [stepUpModal, setStepUpModal] = useState({ open: false, threshold: null, pendingData: null });
  const [isOnline, setIsOnline] = useState(
    typeof navigator !== "undefined" && typeof navigator.onLine === "boolean" ? navigator.onLine : true
  );

  useEffect(() => {
    const handleOnline = () => setIsOnline(true);
    const handleOffline = () => setIsOnline(false);
    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOffline);
    return () => {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOffline);
    };
  }, []);

  // #118 — conversion preview state
  const [rate, setRate] = useState(null);
  const [rateLoading, setRateLoading] = useState(false);
  const [rateError, setRateError] = useState(null);

  const fetchRate = useCallback(async (from, to) => {
    if (!from || !to || from === to) { setRate(null); return; }
    setRateLoading(true);
    setRateError(null);
    try {
      const res = await getExchangeRate(from, to);
      setRate(res.data.rate ?? null);
    } catch {
      setRateError("Could not fetch exchange rate.");
      setRate(null);
    } finally {
      setRateLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRate(formData.currency, formData.target_currency);
  }, [formData.currency, formData.target_currency, fetchRate]);

  const handleChange = (e) => {
    const { name, value } = e.target;
    // #117 — sanitize free-text fields only
    // #357 — Stellar addresses never contain whitespace, so trim as the user types
    const safe = name === "notes" ? sanitizeInput(value) : name === "recipient_account" ? value.trim() : value;
    setFormData((prev) => ({ ...prev, [name]: safe }));
    if (fieldErrors[name]) setFieldErrors((prev) => ({ ...prev, [name]: undefined }));
  };

  const executeSubmit = async (dataToSubmit, extraConfig = {}) => {
    const errors = validateRemittanceForm(dataToSubmit);
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      document.getElementById(Object.keys(errors)[0])?.focus();
      return;
    }

    if (!navigator.onLine) {
      setSubmitError({
        type: "network",
        message: "Connection lost. Please check your internet connection and try again.",
      });
      return;
    }

    setLoading(true);
    setSubmitError(null);
    setResult(null);

    try {
      const response = await sendRemittance(dataToSubmit, extraConfig);
      setResult(response.data);
      setFormData(INITIAL);
      setFieldErrors({});
    } catch (err) {
      if (err?.code === "STEP_UP_2FA_REQUIRED" || err?.requires_2fa) {
        if (err?.code === "STEP_UP_2FA_REQUIRED") {
          setStepUpModal({
            open: true,
            threshold: err.threshold || null,
            pendingData: dataToSubmit,
          });
          return;
        }
      }

      let errorType = "general";
      let errorMsg = "Failed to send remittance.";

      if (!navigator.onLine || err.code === "ERR_NETWORK" || err.message?.includes("Network Error")) {
        errorType = "network";
        errorMsg = "Connection lost. Please check your internet connection and try again.";
      } else if (err.response?.status >= 500 || err.status >= 500) {
        errorType = "server";
        errorMsg = "Server error. Please try again later.";
      } else if (typeof err === "string") {
        errorMsg = err;
      } else if (err.response?.data?.error) {
        errorMsg = err.response.data.error;
      } else if (err.message) {
        errorMsg = err.message;
      }

      setSubmitError({ type: errorType, message: errorMsg });
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = (e) => {
    e.preventDefault();
    executeSubmit(formData);
  };

  const handleRetry = () => {
    executeSubmit(formData);
  };

  return (
    <div className="remittance-form">
      {/* #116 — explicit heading for landmark + form association */}
      <h2 id="remittance-heading">Send Remittance</h2>

      {!isOnline && (
        <div role="alert" className="offline-warning alert-error">
          ⚠️ You are currently offline. Please check your connection.
        </div>
      )}

      <form onSubmit={handleSubmit} noValidate aria-labelledby="remittance-heading">
        <div className="form-group">
          <label htmlFor="sender_id">Sender ID</label>
          <input
            id="sender_id"
            type="number"
            inputMode="numeric"
            name="sender_id"
            value={formData.sender_id}
            onChange={handleChange}
            required
            aria-required="true"
            aria-describedby={fieldErrors.sender_id ? "err-sender_id" : undefined}
            aria-invalid={!!fieldErrors.sender_id}
          />
          <FieldError id="err-sender_id" message={fieldErrors.sender_id} />
        </div>

        <div className="form-group">
          <label htmlFor="recipient_id">Recipient ID</label>
          <input
            id="recipient_id"
            type="number"
            inputMode="numeric"
            name="recipient_id"
            value={formData.recipient_id}
            onChange={handleChange}
            required
            aria-required="true"
            aria-describedby={fieldErrors.recipient_id ? "err-recipient_id" : undefined}
            aria-invalid={!!fieldErrors.recipient_id}
          />
          <FieldError id="err-recipient_id" message={fieldErrors.recipient_id} />
        </div>

        {/* #357 — Stellar address with inline format validation and a valid-state indicator */}
        <div className="form-group">
          <label htmlFor="recipient_account">Recipient Stellar Address</label>
          <div className="input-with-status">
            <input
              id="recipient_account"
              type="text"
              name="recipient_account"
              value={formData.recipient_account}
              onChange={handleChange}
              onBlur={() =>
                setFieldErrors((prev) => ({
                  ...prev,
                  recipient_account: validateStellarAddress(formData.recipient_account) || undefined,
                }))
              }
              placeholder="G..."
              autoComplete="off"
              spellCheck={false}
              required
              aria-required="true"
              aria-describedby={fieldErrors.recipient_account ? "err-recipient_account" : undefined}
              aria-invalid={!!fieldErrors.recipient_account}
            />
            {isValidStellarAddress(formData.recipient_account) && (
              <span className="address-valid" role="img" aria-label="Valid Stellar address">
                {"✓"}
              </span>
            )}
          </div>
          <FieldError id="err-recipient_account" message={fieldErrors.recipient_account} />
        </div>

        <div className="form-group">
          <label htmlFor="amount">Amount</label>
          <input
            id="amount"
            type="number"
            inputMode="decimal"
            name="amount"
            value={formData.amount}
            onChange={handleChange}
            step="0.0000001"
            min="0.01"
            required
            aria-required="true"
            aria-describedby={fieldErrors.amount ? "err-amount" : undefined}
            aria-invalid={!!fieldErrors.amount}
          />
          <FieldError id="err-amount" message={fieldErrors.amount} />
        </div>

        <div className="form-group form-row">
          <div>
            <label htmlFor="currency">From Currency</label>
            <select
              id="currency"
              name="currency"
              value={formData.currency}
              onChange={handleChange}
              aria-describedby={fieldErrors.currency ? "err-currency" : undefined}
              aria-invalid={!!fieldErrors.currency}
            >
              {["USD", "EUR", "GBP", "XLM", "NGN", "KES", "GHS"].map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
            <FieldError id="err-currency" message={fieldErrors.currency} />
          </div>

          <div>
            <label htmlFor="target_currency">To Currency</label>
            <select
              id="target_currency"
              name="target_currency"
              value={formData.target_currency}
              onChange={handleChange}
              aria-describedby={fieldErrors.target_currency ? "err-target_currency" : undefined}
              aria-invalid={!!fieldErrors.target_currency}
            >
              {["USD", "EUR", "GBP", "XLM", "NGN", "KES", "GHS"].map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
            <FieldError id="err-target_currency" message={fieldErrors.target_currency} />
          </div>
        </div>

        {/* #118 — live conversion preview */}
        <ConversionPreview
          amount={formData.amount}
          fromCurrency={formData.currency}
          toCurrency={formData.target_currency}
          rate={rate}
          loading={rateLoading}
          error={rateError}
        />

        <div className="form-group">
          <label htmlFor="notes">
            Notes <span aria-hidden="true">(optional)</span>
          </label>
          <textarea
            id="notes"
            name="notes"
            value={formData.notes}
            onChange={handleChange}
            rows={3}
            maxLength={500}
            aria-describedby={fieldErrors.notes ? "err-notes" : "notes-hint"}
            aria-invalid={!!fieldErrors.notes}
          />
          <span id="notes-hint" className="hint">
            {500 - formData.notes.length} characters remaining
          </span>
          <FieldError id="err-notes" message={fieldErrors.notes} />
        </div>

        <button type="submit" disabled={loading} aria-busy={loading}>
          {loading ? <LoadingSpinner label="Sending…" size={18} /> : "Send Remittance"}
        </button>
      </form>

      {result && (
        <div role="status" className="success" aria-live="polite">
          <h3>Remittance Sent Successfully!</h3>
          <p>Payment ID: {result.id}</p>
          <p>Status: {result.status}</p>
        </div>
      )}

      {submitError && (
        <div role="alert" className="error" aria-live="assertive">
          <h3>{submitError.type === "network" ? "Connection Issue" : "Error"}</h3>
          <p>{submitError.message || submitError}</p>
          <button
            type="button"
            className="retry-btn"
            onClick={handleRetry}
            disabled={loading}
            style={{ marginTop: "8px", padding: "6px 12px", cursor: "pointer" }}
          >
            Retry
          </button>
        </div>
      )}

      <StepUp2FAModal
        isOpen={stepUpModal.open}
        threshold={stepUpModal.threshold}
        onVerified={(stepUpToken) => {
          const pending = stepUpModal.pendingData;
          setStepUpModal({ open: false, threshold: null, pendingData: null });
          if (pending) {
            executeSubmit(pending, { headers: { "X-Step-Up-Token": stepUpToken } });
          }
        }}
        onCancel={() => setStepUpModal({ open: false, threshold: null, pendingData: null })}
      />
    </div>
  );
}

export default RemittanceForm;
