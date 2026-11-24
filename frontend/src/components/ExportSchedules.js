import React, { useEffect, useState } from "react";
import {
  getExportSchedules,
  createExportSchedule,
  updateExportSchedule,
  deleteExportSchedule,
} from "../services/api";
import LoadingSpinner from "./LoadingSpinner";

function ExportSchedules() {
  const [schedules, setSchedules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [form, setForm] = useState({
    frequency: "weekly",
    format: "csv",
    destination: "email",
    webhook_url: "",
  });

  const loadSchedules = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getExportSchedules();
      setSchedules(Array.isArray(res?.data) ? res.data : []);
    } catch (err) {
      setError(err?.message || "Failed to load export schedules.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadSchedules();
  }, []);

  const handleCreate = async (e) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const payload = {
        frequency: form.frequency,
        format: form.format,
        destination: form.destination,
        ...(form.destination === "webhook" && form.webhook_url
          ? { webhook_url: form.webhook_url }
          : {}),
      };
      const res = await createExportSchedule(payload);
      if (res?.data) {
        setSchedules((prev) => [res.data, ...prev]);
      }
      setForm({ frequency: "weekly", format: "csv", destination: "email", webhook_url: "" });
    } catch (err) {
      setError(err?.message || "Failed to create export schedule.");
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggleActive = async (sched) => {
    try {
      const res = await updateExportSchedule(sched.ID || sched.id, {
        is_active: !sched.is_active,
      });
      if (res?.data) {
        setSchedules((prev) =>
          prev.map((item) =>
            (item.ID || item.id) === (sched.ID || sched.id) ? res.data : item
          )
        );
      }
    } catch (err) {
      setError(err?.message || "Failed to update export schedule.");
    }
  };

  const handleDelete = async (id) => {
    try {
      await deleteExportSchedule(id);
      setSchedules((prev) => prev.filter((item) => (item.ID || item.id) !== id));
    } catch (err) {
      setError(err?.message || "Failed to delete export schedule.");
    }
  };

  return (
    <section className="export-schedules" aria-labelledby="export-schedules-heading">
      <h2 id="export-schedules-heading">Scheduled Exports</h2>
      <p>Automatically export transaction history on a recurring schedule.</p>

      {error && (
        <div role="alert" className="error" aria-live="assertive">
          {error}
        </div>
      )}

      <form onSubmit={handleCreate} className="export-schedule-form">
        <div>
          <label htmlFor="export-frequency">Frequency</label>
          <select
            id="export-frequency"
            value={form.frequency}
            onChange={(e) => setForm((p) => ({ ...p, frequency: e.target.value }))}
          >
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
            <option value="monthly">Monthly</option>
          </select>
        </div>

        <div>
          <label htmlFor="export-format">Format</label>
          <select
            id="export-format"
            value={form.format}
            onChange={(e) => setForm((p) => ({ ...p, format: e.target.value }))}
          >
            <option value="csv">CSV</option>
            <option value="pdf">PDF</option>
          </select>
        </div>

        <div>
          <label htmlFor="export-destination">Destination</label>
          <select
            id="export-destination"
            value={form.destination}
            onChange={(e) => setForm((p) => ({ ...p, destination: e.target.value }))}
          >
            <option value="email">Email</option>
            <option value="webhook">Webhook</option>
          </select>
        </div>

        {form.destination === "webhook" && (
          <div>
            <label htmlFor="export-webhook-url">Webhook URL</label>
            <input
              id="export-webhook-url"
              type="url"
              value={form.webhook_url}
              onChange={(e) => setForm((p) => ({ ...p, webhook_url: e.target.value }))}
              placeholder="https://example.com/webhooks/export"
              required
            />
          </div>
        )}

        <button type="submit" disabled={submitting}>
          {submitting ? "Creating…" : "Create Export Schedule"}
        </button>
      </form>

      {loading ? (
        <LoadingSpinner label="Loading export schedules…" size={24} />
      ) : schedules.length === 0 ? (
        <p>No export schedules configured yet.</p>
      ) : (
        <ul className="export-schedule-list">
          {schedules.map((sched) => {
            const id = sched.ID || sched.id;
            return (
              <li key={id} className="export-schedule-item">
                <span>
                  <strong>{sched.frequency}</strong> ({sched.format}) → {sched.destination}
                </span>
                <span>Status: {sched.is_active ? "Active" : "Paused"}</span>
                <button type="button" onClick={() => handleToggleActive(sched)}>
                  {sched.is_active ? "Pause" : "Resume"}
                </button>
                <button type="button" onClick={() => handleDelete(id)}>
                  Delete
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export default ExportSchedules;
