import React, { useEffect, useState } from "react";
import { getEmailPreferences, updateEmailPreferences } from "../services/api";
import LoadingSpinner from "./LoadingSpinner";

function EmailPreferences() {
  const [prefs, setPrefs] = useState({
    transaction_confirmations: true,
    marketing: false,
    security_alerts: true,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    let active = true;
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await getEmailPreferences();
        if (active && res?.data) {
          setPrefs({
            transaction_confirmations: Boolean(res.data.transaction_confirmations),
            marketing: Boolean(res.data.marketing),
            security_alerts: true,
          });
        }
      } catch (err) {
        if (active) {
          setError(err?.message || "Failed to load email preferences.");
        }
      } finally {
        if (active) setLoading(false);
      }
    };
    load();
    return () => {
      active = false;
    };
  }, []);

  const handleToggle = (key) => {
    if (key === "security_alerts") return;
    setPrefs((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    setMessage(null);
    setError(null);
    try {
      const res = await updateEmailPreferences({
        transaction_confirmations: prefs.transaction_confirmations,
        marketing: prefs.marketing,
        security_alerts: true,
      });
      if (res?.data) {
        setPrefs({
          transaction_confirmations: Boolean(res.data.transaction_confirmations),
          marketing: Boolean(res.data.marketing),
          security_alerts: true,
        });
      }
      setMessage("Email preferences saved.");
    } catch (err) {
      setError(err?.message || "Failed to save email preferences.");
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="email-preferences" aria-busy="true">
        <LoadingSpinner label="Loading email preferences…" size={24} />
      </div>
    );
  }

  return (
    <section className="email-preferences" aria-labelledby="email-prefs-heading">
      <h2 id="email-prefs-heading">Email Notification Preferences</h2>
      <p>Choose which email notifications you want to receive.</p>

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

      <form onSubmit={handleSubmit}>
        <div className="preference-item">
          <label htmlFor="pref-transaction">
            <input
              id="pref-transaction"
              type="checkbox"
              checked={prefs.transaction_confirmations}
              onChange={() => handleToggle("transaction_confirmations")}
              disabled={saving}
            />
            <span>Transaction confirmations &amp; payment updates</span>
          </label>
        </div>

        <div className="preference-item">
          <label htmlFor="pref-marketing">
            <input
              id="pref-marketing"
              type="checkbox"
              checked={prefs.marketing}
              onChange={() => handleToggle("marketing")}
              disabled={saving}
            />
            <span>Product updates &amp; marketing emails</span>
          </label>
        </div>

        <div className="preference-item">
          <label htmlFor="pref-security">
            <input
              id="pref-security"
              type="checkbox"
              checked={true}
              disabled={true}
              readOnly
            />
            <span>Security alerts (required — cannot be disabled)</span>
          </label>
        </div>

        <button type="submit" disabled={saving} aria-busy={saving}>
          {saving ? "Saving…" : "Save Preferences"}
        </button>
      </form>
    </section>
  );
}

export default EmailPreferences;
