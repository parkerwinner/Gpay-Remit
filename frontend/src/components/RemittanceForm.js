// #110 form validation  #116 accessibility  #117 input sanitization  #118 conversion preview
import React, { useState, useEffect, useCallback } from "react";
import { sendRemittance, getExchangeRate } from "../services/api";
import { validateRemittanceForm } from "../utils/validation";
import { sanitizeInput } from "../utils/sanitize";
import ConversionPreview from "./ConversionPreview";
import LoadingSpinner from "./LoadingSpinner";

const INITIAL = {
  sender_id: "",
  recipient_id: "",
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
    const safe = name === "notes" ? sanitizeInput(value) : value;
    setFormData((prev) => ({ ...prev, [name]: safe }));
    if (fieldErrors[name]) setFieldErrors((prev) => ({ ...prev, [name]: undefined }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    // #110 — run full validation before sending
    const errors = validateRemittanceForm(formData);
    if (Object.keys(errors).length > 0) {
      setFieldErrors(errors);
      document.getElementById(Object.keys(errors)[0])?.focus();
      return;
    }

    setLoading(true);
    setSubmitError(null);
    setResult(null);

    try {
      const response = await sendRemittance(formData);
      setResult(response.data);
      setFormData(INITIAL);
      setFieldErrors({});
    } catch (err) {
      setSubmitError(err.response?.data?.error || "Failed to send remittance.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="remittance-form">
      {/* #116 — explicit heading for landmark + form association */}
      <h2 id="remittance-heading">Send Remittance</h2>

      <form onSubmit={handleSubmit} noValidate aria-labelledby="remittance-heading">
        <div className="form-group">
          <label htmlFor="sender_id">Sender ID</label>
          <input
            id="sender_id"
            type="number"
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

        <div className="form-group">
          <label htmlFor="amount">Amount</label>
          <input
            id="amount"
            type="number"
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
              {["USD","EUR","GBP","XLM","NGN","KES","GHS"].map((c) => (
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
              {["USD","EUR","GBP","XLM","NGN","KES","GHS"].map((c) => (
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
          <h3>Error</h3>
          <p>{submitError}</p>
        </div>
      )}
    </div>
  );
}

export default RemittanceForm;
