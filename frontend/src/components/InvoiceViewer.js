// #109 skeleton loader  #113 real-time polling  #116 accessibility
import React, { useState, useCallback } from "react";
import { getInvoices } from "../services/api";
import { usePolling } from "../hooks/usePolling";
import SkeletonLoader from "./SkeletonLoader";

function InvoiceViewer() {
  const [invoices, setInvoices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchInvoices = useCallback(async () => {
    try {
      const response = await getInvoices();
      setInvoices(response.data);
      setError(null);
    } catch (err) {
      setError(err.response?.data?.error || "Failed to fetch invoices.");
    } finally {
      setLoading(false);
    }
  }, []);

  // #113 — poll every 10 s so status updates surface without a manual refresh.
  usePolling(fetchInvoices, 10_000);

  return (
    <div className="invoice-viewer">
      <h2 id="invoices-heading">Invoices</h2>

      {loading && <SkeletonLoader rows={4} columns={5} />}

      {error && (
        <p role="alert" className="error" aria-live="assertive">
          {error}
        </p>
      )}

      {!loading && !error && (
        invoices.length === 0 ? (
          <p>No invoices found.</p>
        ) : (
          <table aria-labelledby="invoices-heading">
            <thead>
              <tr>
                <th scope="col">Invoice No</th>
                <th scope="col">Amount</th>
                <th scope="col">Currency</th>
                <th scope="col">Status</th>
                <th scope="col">Created</th>
              </tr>
            </thead>
            <tbody>
              {invoices.map((invoice) => (
                <tr key={invoice.id}>
                  <td>{invoice.invoice_no}</td>
                  <td>{invoice.amount}</td>
                  <td>{invoice.currency}</td>
                  <td>
                    <span className={`status-badge status-${invoice.status?.toLowerCase()}`}>
                      {invoice.status}
                    </span>
                  </td>
                  <td>{new Date(invoice.created_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )
      )}
    </div>
  );
}

export default InvoiceViewer;
