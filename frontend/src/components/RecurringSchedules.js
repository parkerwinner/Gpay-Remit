import React, { useState, useEffect, useCallback } from "react";
import {
  getRecurringSchedules,
  createRecurringSchedule,
  pauseRecurringSchedule,
  resumeRecurringSchedule,
  cancelRecurringSchedule,
} from "../services/api";
import { validateStellarAddress } from "../utils/validation";
import { sanitizeInput } from "../utils/sanitize";

const CURRENCIES = ["USD", "EUR", "GBP", "XLM", "USDC", "NGN", "KES", "GHS"];

const INITIAL_FORM = {
  sender_account: "",
  recipient_account: "",
  amount: "",
  currency: "USDC",
  frequency: "monthly",
  next_run_at: "",
  notes: "",
};

function RecurringSchedules() {
  const [schedules, setSchedules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [formData, setFormData] = useState(INITIAL_FORM);
  const [formError, setFormError] = useState(null);
  const [pendingConfirm, setPendingConfirm] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  const loadSchedules = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getRecurringSchedules();
      setSchedules(res.data?.data || []);
    } catch (err) {
      setError(err?.message || "Failed to load recurring schedules.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadSchedules();
  }, [loadSchedules]);

  const handleChange = (e) => {
    const { name, value } = e.target;
    const cleaned =
      name === "notes"
        ? sanitizeInput(value)
        : name === "sender_account" || name === "recipient_account"
        ? value.trim()
        : value;
    setFormData((prev) => ({ ...prev, [name]: cleaned }));
  };

  const handleReview = (e) => {
    e.preventDefault();
    const senderErr = validateStellarAddress(formData.sender_account);
    if (senderErr) {
      setFormError(`Sender: ${senderErr}`);
      return;
    }
    const recipErr = validateStellarAddress(formData.recipient_account);
    if (recipErr) {
      setFormError(`Recipient: ${recipErr}`);
      return;
    }
    const amt = parseFloat(formData.amount);
    if (isNaN(amt) || amt <= 0) {
      setFormError("Amount must be greater than zero.");
      return;
    }
    setFormError(null);
    setPendingConfirm({
      ...formData,
      amountUnits: Math.round(amt * 10000000),
      displayAmount: amt,
    });
  };

  const handleConfirmCreate = async () => {
    if (!pendingConfirm || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      const payload = {
        sender_account: pendingConfirm.sender_account,
        recipient_account: pendingConfirm.recipient_account,
        amount: pendingConfirm.amountUnits,
        currency: pendingConfirm.currency,
        frequency: pendingConfirm.frequency,
        notes: pendingConfirm.notes,
      };
      if (pendingConfirm.next_run_at) {
        payload.next_run_at = new Date(pendingConfirm.next_run_at).toISOString();
      }
      await createRecurringSchedule(payload);
      setPendingConfirm(null);
      setFormData(INITIAL_FORM);
      await loadSchedules();
    } catch (err) {
      setError(err?.message || "Failed to create recurring schedule.");
    } finally {
      setSubmitting(false);
    }
  };

  const handleAction = async (actionFn, id) => {
    setError(null);
    try {
      await actionFn(id);
      await loadSchedules();
    } catch (err) {
      setError(err?.message || "Failed to update schedule.");
    }
  };

  return (
    <section className="feature-card" aria-labelledby="recurring-heading">
      <h2 id="recurring-heading">Recurring Remittances</h2>

      {error && (
        <div role="alert" className="alert-error">
          {error}
        </div>
      )}

      <form onSubmit={handleReview} aria-label="Create recurring schedule">
        <h3>New Recurring Schedule</h3>
        {formError && (
          <div role="alert" className="field-error">
            {formError}
          </div>
        )}
        <div className="form-group">
          <label htmlFor="rec_sender">Sender Stellar Address</label>
          <input
            id="rec_sender"
            name="sender_account"
            value={formData.sender_account}
            onChange={handleChange}
            autoComplete="off"
            spellCheck={false}
            placeholder="G..."
          />
        </div>
        <div className="form-group">
          <label htmlFor="rec_recipient">Recipient Stellar Address</label>
          <input
            id="rec_recipient"
            name="recipient_account"
            value={formData.recipient_account}
            onChange={handleChange}
            autoComplete="off"
            spellCheck={false}
            placeholder="G..."
          />
        </div>
        <div className="form-row">
          <div className="form-group">
            <label htmlFor="rec_amount">Amount</label>
            <input
              id="rec_amount"
              name="amount"
              type="number"
              inputMode="decimal"
              step="any"
              value={formData.amount}
              onChange={handleChange}
            />
          </div>
          <div className="form-group">
            <label htmlFor="rec_currency">Currency</label>
            <select id="rec_currency" name="currency" value={formData.currency} onChange={handleChange}>
              {CURRENCIES.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label htmlFor="rec_frequency">Frequency</label>
            <select id="rec_frequency" name="frequency" value={formData.frequency} onChange={handleChange}>
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
            </select>
          </div>
        </div>
        <div className="form-group">
          <label htmlFor="rec_next_run">First Run Date (UTC, optional)</label>
          <input
            id="rec_next_run"
            name="next_run_at"
            type="datetime-local"
            value={formData.next_run_at}
            onChange={handleChange}
          />
        </div>
        <button type="submit">Review Schedule</button>
      </form>

      {pendingConfirm && (
        <div className="confirm-box" role="dialog" aria-label="Confirm recurring schedule">
          <h3>Confirm Recurring Schedule</h3>
          <p>Recipient: {pendingConfirm.recipient_account}</p>
          <p>
            Amount: {pendingConfirm.displayAmount} {pendingConfirm.currency}
          </p>
          <p>Frequency: {pendingConfirm.frequency}</p>
          <p>
            First Run:{" "}
            {pendingConfirm.next_run_at
              ? new Date(pendingConfirm.next_run_at).toUTCString()
              : "Next scheduled interval (UTC)"}
          </p>
          <div className="button-row">
            <button type="button" onClick={handleConfirmCreate} disabled={submitting}>
              {submitting ? "Creating..." : "Confirm Schedule"}
            </button>
            <button type="button" className="secondary-btn" onClick={() => setPendingConfirm(null)} disabled={submitting}>
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="list-section">
        <h3>Your Schedules</h3>
        {loading ? (
          <p>Loading schedules...</p>
        ) : schedules.length === 0 ? (
          <p>No recurring schedules configured.</p>
        ) : (
          <ul className="item-list">
            {schedules.map((s) => (
              <li key={s.id} className="item-card">
                <div>
                  <strong>
                    {(s.amount / 10000000).toFixed(2)} {s.currency} ({s.frequency})
                  </strong>
                  <div className="item-meta">
                    Status: {s.status} | To: {s.recipient_account}
                  </div>
                  <div className="item-meta">
                    Next run (UTC): {new Date(s.next_run_at).toISOString()}
                  </div>
                </div>
                <div className="button-row">
                  {s.status === "active" && (
                    <button type="button" className="secondary-btn" onClick={() => handleAction(pauseRecurringSchedule, s.id)}>
                      Pause
                    </button>
                  )}
                  {s.status === "paused" && (
                    <button type="button" onClick={() => handleAction(resumeRecurringSchedule, s.id)}>
                      Resume
                    </button>
                  )}
                  {s.status !== "cancelled" && (
                    <button type="button" className="danger-btn" onClick={() => handleAction(cancelRecurringSchedule, s.id)}>
                      Cancel
                    </button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

export default RecurringSchedules;
