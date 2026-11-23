import React, { useState, useEffect, useCallback } from "react";
import {
  getTemplates,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  sendFromTemplate,
  calculateFee,
} from "../services/api";
import { validateStellarAddress } from "../utils/validation";
import { sanitizeInput } from "../utils/sanitize";

const CURRENCIES = ["USD", "EUR", "GBP", "XLM", "USDC", "NGN", "KES", "GHS"];

const EMPTY_FORM = {
  template_name: "",
  recipient_account: "",
  amount: "",
  asset_code: "USDC",
  notes: "",
};

function PaymentTemplates() {
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [formData, setFormData] = useState(EMPTY_FORM);
  const [editingId, setEditingId] = useState(null);
  const [formError, setFormError] = useState(null);
  const [saving, setSaving] = useState(false);

  const [deleteTarget, setDeleteTarget] = useState(null);
  const [sendTarget, setSendTarget] = useState(null);
  const [estimatedFee, setEstimatedFee] = useState(null);
  const [sending, setSending] = useState(false);
  const [sendResult, setSendResult] = useState(null);

  const loadTemplates = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getTemplates();
      setTemplates(res.data?.data || []);
    } catch (err) {
      setError(err?.message || "Failed to load payment templates.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadTemplates();
  }, [loadTemplates]);

  const handleChange = (e) => {
    const { name, value } = e.target;
    const cleaned =
      name === "notes" || name === "template_name"
        ? sanitizeInput(value)
        : name === "recipient_account"
        ? value.trim()
        : value;
    setFormData((prev) => ({ ...prev, [name]: cleaned }));
  };

  const validateForm = () => {
    if (!formData.template_name.trim()) return "Template name is required.";
    const addrErr = validateStellarAddress(formData.recipient_account);
    if (addrErr) return addrErr;
    const amt = parseFloat(formData.amount);
    if (isNaN(amt) || amt <= 0) return "Amount must be greater than zero.";
    if (!CURRENCIES.includes(formData.asset_code)) return "Unsupported currency.";
    return null;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    const validationErr = validateForm();
    if (validationErr) {
      setFormError(validationErr);
      return;
    }
    setSaving(true);
    setFormError(null);
    try {
      const payload = {
        ...formData,
        amount: parseFloat(formData.amount),
      };
      if (editingId) {
        await updateTemplate(editingId, payload);
      } else {
        await createTemplate(payload);
      }
      setFormData(EMPTY_FORM);
      setEditingId(null);
      await loadTemplates();
    } catch (err) {
      setFormError(err?.message || "Failed to save template.");
    } finally {
      setSaving(false);
    }
  };

  const startEdit = (tpl) => {
    setEditingId(tpl.id);
    setFormData({
      template_name: tpl.template_name || "",
      recipient_account: tpl.recipient_account || "",
      amount: String(tpl.amount ?? ""),
      asset_code: tpl.asset_code || "USDC",
      notes: tpl.notes || "",
    });
    setFormError(null);
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteTemplate(deleteTarget.id);
      setDeleteTarget(null);
      await loadTemplates();
    } catch (err) {
      setError(err?.message || "Failed to delete template.");
    }
  };

  const openSendConfirm = async (tpl) => {
    setSendTarget(tpl);
    setSendResult(null);
    setEstimatedFee(null);
    try {
      const units = Math.round(Number(tpl.amount) * 10000000);
      const res = await calculateFee(units);
      setEstimatedFee(res.data?.total_fee ?? null);
    } catch {
      setEstimatedFee(null);
    }
  };

  const confirmSend = async () => {
    if (!sendTarget || sending) return;
    setSending(true);
    setError(null);
    try {
      const res = await sendFromTemplate(sendTarget.id);
      setSendResult(res.data);
      setSendTarget(null);
    } catch (err) {
      setError(err?.message || "Failed to send remittance from template.");
    } finally {
      setSending(false);
    }
  };

  return (
    <section className="feature-card" aria-labelledby="templates-heading">
      <h2 id="templates-heading">Payment Templates</h2>

      {error && (
        <div role="alert" className="alert-error">
          {error}
        </div>
      )}
      {sendResult && (
        <div role="status" className="alert-success">
          Remittance #{sendResult.remittance_id} created ({sendResult.status}).
        </div>
      )}

      <form onSubmit={handleSubmit} className="template-form" aria-label="Payment template form">
        <h3>{editingId ? "Edit Template" : "Create Template"}</h3>
        {formError && (
          <div role="alert" className="field-error">
            {formError}
          </div>
        )}
        <div className="form-group">
          <label htmlFor="template_name">Template Name</label>
          <input
            id="template_name"
            name="template_name"
            value={formData.template_name}
            onChange={handleChange}
            placeholder="e.g. Monthly Rent"
          />
        </div>
        <div className="form-group">
          <label htmlFor="tpl_recipient_account">Recipient Stellar Address</label>
          <input
            id="tpl_recipient_account"
            name="recipient_account"
            value={formData.recipient_account}
            onChange={handleChange}
            placeholder="G..."
          />
        </div>
        <div className="form-row">
          <div className="form-group">
            <label htmlFor="tpl_amount">Amount</label>
            <input
              id="tpl_amount"
              name="amount"
              type="number"
              step="any"
              value={formData.amount}
              onChange={handleChange}
            />
          </div>
          <div className="form-group">
            <label htmlFor="tpl_asset_code">Currency</label>
            <select
              id="tpl_asset_code"
              name="asset_code"
              value={formData.asset_code}
              onChange={handleChange}
            >
              {CURRENCIES.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="form-group">
          <label htmlFor="tpl_notes">Notes</label>
          <input
            id="tpl_notes"
            name="notes"
            value={formData.notes}
            onChange={handleChange}
          />
        </div>
        <div className="button-row">
          <button type="submit" disabled={saving}>
            {saving ? "Saving..." : editingId ? "Update Template" : "Save Template"}
          </button>
          {editingId && (
            <button
              type="button"
              className="secondary-btn"
              onClick={() => {
                setEditingId(null);
                setFormData(EMPTY_FORM);
              }}
            >
              Cancel Edit
            </button>
          )}
        </div>
      </form>

      {deleteTarget && (
        <div className="confirm-box" role="dialog" aria-label="Confirm delete template">
          <p>
            Delete template <strong>{deleteTarget.template_name}</strong>?
          </p>
          <div className="button-row">
            <button type="button" onClick={confirmDelete}>
              Confirm Delete
            </button>
            <button type="button" className="secondary-btn" onClick={() => setDeleteTarget(null)}>
              Cancel
            </button>
          </div>
        </div>
      )}

      {sendTarget && (
        <div className="confirm-box" role="dialog" aria-label="Confirm send from template">
          <h3>Confirm Send</h3>
          <p>Recipient: {sendTarget.recipient_account}</p>
          <p>
            Amount: {sendTarget.amount} {sendTarget.asset_code}
          </p>
          <p>
            Estimated Fee:{" "}
            {estimatedFee !== null ? `${(estimatedFee / 10000000).toFixed(4)} ${sendTarget.asset_code}` : "Calculated on send"}
          </p>
          <div className="button-row">
            <button type="button" onClick={confirmSend} disabled={sending}>
              {sending ? "Sending..." : "Confirm & Send"}
            </button>
            <button type="button" className="secondary-btn" onClick={() => setSendTarget(null)} disabled={sending}>
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="list-section">
        <h3>Saved Templates</h3>
        {loading ? (
          <p>Loading templates...</p>
        ) : templates.length === 0 ? (
          <p>No payment templates saved yet.</p>
        ) : (
          <ul className="item-list">
            {templates.map((tpl) => (
              <li key={tpl.id} className="item-card">
                <div>
                  <strong>{tpl.template_name}</strong>
                  <div className="item-meta">
                    {tpl.amount} {tpl.asset_code} → {tpl.recipient_account}
                  </div>
                </div>
                <div className="button-row">
                  <button type="button" onClick={() => openSendConfirm(tpl)}>
                    Send
                  </button>
                  <button type="button" className="secondary-btn" onClick={() => startEdit(tpl)}>
                    Edit
                  </button>
                  <button type="button" className="danger-btn" onClick={() => setDeleteTarget(tpl)}>
                    Delete
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

export default PaymentTemplates;
