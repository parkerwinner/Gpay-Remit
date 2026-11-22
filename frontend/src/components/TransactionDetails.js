import { formatMinorUnits } from '../utils/currency';
// #112 — modal/detail view for a single transaction record.
import React, { useEffect } from "react";

function TransactionDetails({ transaction, onClose }) {
  useEffect(() => {
    if (!transaction) return undefined;

    const handleKeyDown = (event) => {
      if (event.key === "Escape") onClose();
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [transaction, onClose]);

  if (!transaction) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="tx-detail-title"
      className="transaction-details-overlay"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="transaction-details-card">
        <button
          type="button"
          className="close-btn"
          onClick={onClose}
          aria-label="Close transaction details"
        >
          ✕
        </button>
        <h3 id="tx-detail-title">Transaction #{transaction.id}</h3>
        <dl className="detail-list">
          <dt>Sender ID</dt>
          <dd>{transaction.sender_id}</dd>
          <dt>Recipient ID</dt>
          <dd>{transaction.recipient_id}</dd>
          <dt>Amount</dt>
          <dd>
            {formatMinorUnits(transaction.amount, transaction.currency)} {transaction.currency}
          </dd>
          {transaction.converted_amount && (
            <>
              <dt>Converted</dt>
              <dd>
                {formatMinorUnits(transaction.converted_amount, transaction.target_currency)} {transaction.target_currency}
              </dd>
            </>
          )}
          <dt>Status</dt>
          <dd>
            <span className={`status-badge status-${transaction.status?.toLowerCase()}`}>
              {transaction.status}
            </span>
          </dd>
          {transaction.notes && (
            <>
              <dt>Notes</dt>
              <dd>{transaction.notes}</dd>
            </>
          )}
          <dt>Created</dt>
          <dd>{new Date(transaction.created_at).toLocaleString()}</dd>
        </dl>
      </div>
    </div>
  );
}

export default TransactionDetails;
