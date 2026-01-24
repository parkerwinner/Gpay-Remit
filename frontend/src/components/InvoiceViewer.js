import React, { useState, useEffect } from "react";
import { getInvoices } from "../services/api";

function InvoiceViewer() {
  const [invoices, setInvoices] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchInvoices();
  }, []);

  const fetchInvoices = async () => {
    try {
      const response = await getInvoices();
      setInvoices(response.data);
    } catch (err) {
      setError(err.response?.data?.error || "Failed to fetch invoices");
    } finally {
      setLoading(false);
    }
  };

  if (loading) return <div>Loading invoices...</div>;
  if (error) return <div className="error">{error}</div>;

  return (
    <div className="invoice-viewer">
      <h2>Invoices</h2>
      {invoices.length === 0 ? (
        <p>No invoices found</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Invoice No</th>
              <th>Amount</th>
              <th>Currency</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {invoices.map((invoice) => (
              <tr key={invoice.id}>
                <td>{invoice.invoice_no}</td>
                <td>{invoice.amount}</td>
                <td>{invoice.currency}</td>
                <td>{invoice.status}</td>
                <td>{new Date(invoice.created_at).toLocaleDateString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export default InvoiceViewer;
