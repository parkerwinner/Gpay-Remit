// #455: Tag summary dashboard showing per-tag counts and per-asset totals.
import React from "react";
import { formatMinorUnits } from "../utils/currency";

function TagSummaryDashboard({ summary = [], loading = false, error = null }) {
  return (
    <section className="tag-summary-card" aria-labelledby="tag-summary-heading">
      <h3 id="tag-summary-heading">Tag Summary (Completed Payments)</h3>

      {loading && <p className="item-meta">Loading tag summary…</p>}
      {error && (
        <p role="alert" className="alert-error">
          {error}
        </p>
      )}

      {!loading && !error && summary.length === 0 && (
        <p className="item-meta">No tagged completed payments yet.</p>
      )}

      {!loading && !error && summary.length > 0 && (
        <div className="table-responsive">
          <table aria-labelledby="tag-summary-heading">
            <thead>
              <tr>
                <th scope="col">Tag</th>
                <th scope="col">Completed Payments</th>
                <th scope="col">Totals by Asset</th>
              </tr>
            </thead>
            <tbody>
              {summary.map((item) => (
                <tr key={item.tag}>
                  <td>
                    <span className="tag-chip">{item.tag}</span>
                  </td>
                  <td>{item.payment_count}</td>
                  <td>
                    {(item.totals || []).map((t) => (
                      <span key={t.asset} className="tag-asset-total">
                        {formatMinorUnits(t.total, t.asset)} {t.asset}
                      </span>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

export default TagSummaryDashboard;
