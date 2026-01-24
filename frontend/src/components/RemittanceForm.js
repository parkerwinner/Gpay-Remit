import React, { useState } from "react";
import { sendRemittance } from "../services/api";

function RemittanceForm() {
  const [formData, setFormData] = useState({
    sender_id: "",
    recipient_id: "",
    amount: "",
    currency: "USD",
    target_currency: "EUR",
    notes: "",
  });
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);

  const handleChange = (e) => {
    setFormData({
      ...formData,
      [e.target.name]: e.target.value,
    });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const response = await sendRemittance(formData);
      setResult(response.data);
      setFormData({
        sender_id: "",
        recipient_id: "",
        amount: "",
        currency: "USD",
        target_currency: "EUR",
        notes: "",
      });
    } catch (err) {
      setError(err.response?.data?.error || "Failed to send remittance");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="remittance-form">
      <h2>Send Remittance</h2>
      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label>Sender ID:</label>
          <input
            type="number"
            name="sender_id"
            value={formData.sender_id}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Recipient ID:</label>
          <input
            type="number"
            name="recipient_id"
            value={formData.recipient_id}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Amount:</label>
          <input
            type="number"
            name="amount"
            value={formData.amount}
            onChange={handleChange}
            step="0.01"
            min="0.01"
            required
          />
        </div>

        <div className="form-group">
          <label>From Currency:</label>
          <select
            name="currency"
            value={formData.currency}
            onChange={handleChange}
          >
            <option value="USD">USD</option>
            <option value="EUR">EUR</option>
            <option value="GBP">GBP</option>
            <option value="XLM">XLM</option>
          </select>
        </div>

        <div className="form-group">
          <label>To Currency:</label>
          <select
            name="target_currency"
            value={formData.target_currency}
            onChange={handleChange}
          >
            <option value="USD">USD</option>
            <option value="EUR">EUR</option>
            <option value="GBP">GBP</option>
            <option value="XLM">XLM</option>
          </select>
        </div>

        <div className="form-group">
          <label>Notes:</label>
          <textarea
            name="notes"
            value={formData.notes}
            onChange={handleChange}
            rows="3"
          />
        </div>

        <button type="submit" disabled={loading}>
          {loading ? "Sending..." : "Send Remittance"}
        </button>
      </form>

      {result && (
        <div className="success">
          <h3>Remittance Sent Successfully!</h3>
          <p>Payment ID: {result.id}</p>
          <p>Status: {result.status}</p>
        </div>
      )}

      {error && (
        <div className="error">
          <h3>Error</h3>
          <p>{error}</p>
        </div>
      )}
    </div>
  );
}

export default RemittanceForm;
