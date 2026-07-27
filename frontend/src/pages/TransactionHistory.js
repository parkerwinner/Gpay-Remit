// #112 — transaction history page with filtering and detail view.
import React, { useState, useCallback } from "react";
import { getRemittances } from "../services/api";
import { usePolling } from "../hooks/usePolling";
import SkeletonLoader from "../components/SkeletonLoader";
import TransactionDetails from "../components/TransactionDetails";

const STATUS_OPTIONS = ["all", "pending", "completed", "failed"];
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

function TransactionHistory() {
  const [transactions, setTransactions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filter, setFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_OPTIONS[1]);

  const fetchTransactions = useCallback(async (targetPage = page, targetPageSize = pageSize) => {
    setLoading(true);
    try {
      const response = await getRemittances(targetPage, targetPageSize);
      setTransactions(response.data);
      setError(null);
    } catch (err) {
      setError(err.response?.data?.error || "Failed to load transactions.");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  // Poll every 10 s for real-time updates (#113).
  usePolling(fetchTransactions, 10_000);

  const hasNextPage = transactions.length === pageSize;
  const hasPrevPage = page > 1;

  const goToPage = (newPage) => {
    if (newPage < 1) return;
    setPage(newPage);
    fetchTransactions(newPage, pageSize);
  };

  const changePageSize = (newSize) => {
    setPageSize(newSize);
    setPage(1);
    fetchTransactions(1, newSize);
  };

  const filtered = transactions.filter((tx) => {
    const matchesStatus = filter === "all" || tx.status?.toLowerCase() === filter;
    const term = search.toLowerCase();
    const matchesSearch =
      !term ||
      String(tx.id).includes(term) ||
      String(tx.sender_id).includes(term) ||
      String(tx.recipient_id).includes(term);
    return matchesStatus && matchesSearch;
  });

  return (
    <main className="transaction-history" aria-label="Transaction history">
      <h2 id="tx-history-heading">Transaction History</h2>

      <div className="tx-filters" role="search" aria-label="Filter transactions">
        <label htmlFor="tx-search" className="sr-only">
          Search by ID or user
        </label>
        <input
          id="tx-search"
          type="search"
          placeholder="Search by ID or user…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search transactions"
        />

        <label htmlFor="tx-status" className="sr-only">
          Filter by status
        </label>
        <select
          id="tx-status"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          aria-label="Filter by status"
        >
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {s.charAt(0).toUpperCase() + s.slice(1)}
            </option>
          ))}
        </select>
      </div>

      {loading && <SkeletonLoader rows={5} columns={6} />}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}

      {!loading && !error && (
        <table aria-labelledby="tx-history-heading">
          <thead>
            <tr>
              <th scope="col">ID</th>
              <th scope="col">Sender</th>
              <th scope="col">Recipient</th>
              <th scope="col">Amount</th>
              <th scope="col">Status</th>
              <th scope="col">Date</th>
              <th scope="col">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={7}>No transactions found.</td>
              </tr>
            ) : (
              filtered.map((tx) => (
                <tr key={tx.id}>
                  <td>{tx.id}</td>
                  <td>{tx.sender_id}</td>
                  <td>{tx.recipient_id}</td>
                  <td>
                    {tx.amount} {tx.currency}
                  </td>
                  <td>
                    <span className={`status-badge status-${tx.status?.toLowerCase()}`}>
                      {tx.status}
                    </span>
                  </td>
                  <td>{new Date(tx.created_at).toLocaleDateString()}</td>
                  <td>
                    <button
                      onClick={() => setSelected(tx)}
                      aria-label={`View details for transaction ${tx.id}`}
                    >
                      Details
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      )}

      {!error && (
        <div className="tx-pagination" role="navigation" aria-label="Transaction pagination">
          <span className="tx-pagination-status" aria-live="polite">
            Page {page} · {transactions.length} result{transactions.length === 1 ? "" : "s"}
          </span>

          <label htmlFor="tx-page-size" className="sr-only">
            Results per page
          </label>
          <select
            id="tx-page-size"
            value={pageSize}
            onChange={(e) => changePageSize(Number(e.target.value))}
            aria-label="Results per page"
            disabled={loading}
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                {size} / page
              </option>
            ))}
          </select>

          <button
            type="button"
            onClick={() => goToPage(page - 1)}
            disabled={!hasPrevPage || loading}
            aria-label="Previous page"
          >
            Previous
          </button>
          <button
            type="button"
            onClick={() => goToPage(page + 1)}
            disabled={!hasNextPage || loading}
            aria-label="Next page"
          >
            Next
          </button>
        </div>
      )}

      {selected && (
        <TransactionDetails transaction={selected} onClose={() => setSelected(null)} />
      )}
    </main>
  );
}

export default TransactionHistory;
