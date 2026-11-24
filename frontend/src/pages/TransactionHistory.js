import { formatMinorUnits } from '../utils/currency';
// #112 — transaction history page with filtering and detail view.
// #358 — pagination controls with total count and current page indicator.
// #455 — payment tags, tag filtering, tag summary dashboard, and tag autocomplete.
import React, { useState, useCallback, useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { getRemittances, getTagSummary, updateRemittanceTags } from "../services/api";
import { usePolling } from "../hooks/usePolling";
import SkeletonLoader from "../components/SkeletonLoader";
import TransactionDetails from "../components/TransactionDetails";
import TagInputCombobox from "../components/TagInputCombobox";
import TagSummaryDashboard from "../components/TagSummaryDashboard";

const STATUS_OPTIONS = ["all", "pending", "completed", "failed"];
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

function TransactionHistory() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [transactions, setTransactions] = useState([]);
  const [totalCount, setTotalCount] = useState(null); // null = unknown
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filter, setFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(PAGE_SIZE_OPTIONS[1]);
  const [selectedTags, setSelectedTags] = useState(() => searchParams.getAll("tag"));
  const [tagSummary, setTagSummary] = useState([]);
  const [summaryLoading, setSummaryLoading] = useState(true);
  const [summaryError, setSummaryError] = useState(null);
  const [editingTagsTxId, setEditingTagsTxId] = useState(null);
  const [tagSaveError, setTagSaveError] = useState(null);

  const fetchSummary = useCallback(async () => {
    setSummaryLoading(true);
    try {
      const res = await getTagSummary();
      setTagSummary(res.data?.tags ?? []);
      setSummaryError(null);
    } catch (err) {
      setSummaryError(err?.message || err?.response?.data?.error || "Failed to load tag summary.");
    } finally {
      setSummaryLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSummary();
  }, [fetchSummary]);

  const fetchTransactions = useCallback(async (targetPage = page, targetPageSize = pageSize, targetTags = selectedTags) => {
    setLoading(true);
    try {
      const response = await getRemittances(targetPage, targetPageSize, {}, targetTags);

      // Handle both paginated shape { data: [], total: N } and plain array
      if (response.data && typeof response.data === "object" && !Array.isArray(response.data)) {
        setTransactions(response.data.data ?? []);
        setTotalCount(
          typeof response.data.total_count === "number"
            ? response.data.total_count
            : typeof response.data.total === "number"
            ? response.data.total
            : null
        );
      } else {
        setTransactions(response.data ?? []);
        setTotalCount(null);
      }

      setError(null);
    } catch (err) {
      setError(err.response?.data?.error || "Failed to load transactions.");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, selectedTags]);

  const handleTagFilterChange = (nextTags) => {
    setSelectedTags(nextTags);
    setPage(1);
    const nextParams = new URLSearchParams(searchParams);
    nextParams.delete("tag");
    nextTags.forEach((t) => nextParams.append("tag", t));
    setSearchParams(nextParams, { replace: true });
    fetchTransactions(1, pageSize, nextTags);
  };

  const handleSavePaymentTags = async (txId, nextTags) => {
    try {
      setTagSaveError(null);
      const res = await updateRemittanceTags(txId, nextTags);
      const savedTags = res.data?.tags ?? nextTags;
      setTransactions((prev) =>
        prev.map((item) => (item.id === txId ? { ...item, tags: savedTags } : item))
      );
      fetchSummary();
    } catch (err) {
      setTagSaveError(err?.message || err?.response?.data?.error || "Failed to update tags.");
    }
  };

  // Poll every 10 s for real-time updates (#113).
  usePolling(fetchTransactions, 10_000);

  const hasNextPage =
    totalCount !== null
      ? page * pageSize < totalCount
      : transactions.length === pageSize;
  const hasPrevPage = page > 1;
  const totalPages = totalCount !== null ? Math.ceil(totalCount / pageSize) : null;

  const goToPage = (newPage) => {
    if (newPage < 1) return;
    if (totalPages !== null && newPage > totalPages) return;
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

  const pageStart = (page - 1) * pageSize + 1;
  const pageEnd = Math.min(page * pageSize, totalCount ?? page * pageSize);

  return (
    <main className="transaction-history" aria-label="Transaction history">
      <h2 id="tx-history-heading">Transaction History</h2>

      <div className="tx-filters" role="search" aria-label="Filter transactions">
        <label htmlFor="tx-search" className="sr-only">Search by ID or user</label>
        <input id="tx-search" type="search" placeholder="Search by ID or user…" value={search}
          onChange={(e) => setSearch(e.target.value)} aria-label="Search transactions" />
        <label htmlFor="tx-status" className="sr-only">Filter by status</label>
        <select id="tx-status" value={filter} onChange={(e) => setFilter(e.target.value)} aria-label="Filter by status">
          {STATUS_OPTIONS.map((s) => <option key={s} value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>)}
        </select>
      </div>

      <div className="tag-filter-bar">
        <TagInputCombobox
          id="tx-tag-filter"
          label="Filter by tags"
          placeholder="Type a tag to filter…"
          tags={selectedTags}
          onChange={handleTagFilterChange}
        />
      </div>

      {tagSaveError && <p role="alert" className="alert-error">{tagSaveError}</p>}

      <TagSummaryDashboard summary={tagSummary} loading={summaryLoading} error={summaryError} />

      {loading && <SkeletonLoader rows={5} columns={7} />}
      {error && <p role="alert" className="error">{error}</p>}

      {!loading && !error && (
        <div className="table-responsive">
          <table aria-labelledby="tx-history-heading">
          <thead>
            <tr>
              <th scope="col">ID</th><th scope="col">Sender</th><th scope="col">Recipient</th>
              <th scope="col">Amount</th><th scope="col">Status</th><th scope="col">Tags</th><th scope="col">Date</th>
              <th scope="col"><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr><td colSpan={8}>No transactions found.</td></tr>
            ) : (
              filtered.map((tx) => (
                <tr
                  key={tx.id}
                  className="transaction-row"
                  tabIndex={0}
                  role="button"
                  aria-label={`View details for transaction ${tx.id}`}
                  onClick={() => setSelected(tx)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      setSelected(tx);
                    }
                  }}
                >
                  <td>{tx.id}</td><td>{tx.sender_id}</td><td>{tx.recipient_id}</td>
                  <td>{formatMinorUnits(tx.amount, tx.currency)} {tx.currency}</td>
                  <td><span className={`status-badge status-${tx.status?.toLowerCase()}`}>{tx.status}</span></td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <div className="tag-chip-list">
                      {(tx.tags || []).map((tag) => (
                        <span key={tag} className="tag-chip">{tag}</span>
                      ))}
                    </div>
                    {editingTagsTxId === tx.id ? (
                      <TagInputCombobox
                        id={`tx-edit-tags-${tx.id}`}
                        label={`Edit tags for transaction ${tx.id}`}
                        tags={tx.tags || []}
                        onChange={(nextTags) => handleSavePaymentTags(tx.id, nextTags)}
                      />
                    ) : (
                      <button
                        type="button"
                        className="secondary-btn tag-edit-btn"
                        onClick={() => setEditingTagsTxId(tx.id)}
                        aria-label={`Edit tags for transaction ${tx.id}`}
                      >
                        Edit tags
                      </button>
                    )}
                  </td>
                  <td>{new Date(tx.created_at).toLocaleDateString()}</td>
                  <td>
                    <button type="button" onClick={(event) => { event.stopPropagation(); setSelected(tx); }}
                      aria-label={`View details for transaction ${tx.id}`}>Details</button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
        </div>
      )}

      {!error && (
        <div className="tx-pagination" role="navigation" aria-label="Transaction pagination">
          <span className="tx-pagination-status" aria-live="polite">
            {totalCount !== null ? (
              <>{pageStart}–{pageEnd} of {totalCount.toLocaleString()} transaction{totalCount === 1 ? "" : "s"}
                {totalPages !== null && <> · Page {page} of {totalPages}</>}</>
            ) : (
              <>Page {page} · {transactions.length} result{transactions.length === 1 ? "" : "s"}</>
            )}
          </span>
          <label htmlFor="tx-page-size" className="sr-only">Results per page</label>
          <select id="tx-page-size" value={pageSize} onChange={(e) => changePageSize(Number(e.target.value))}
            aria-label="Results per page" disabled={loading}>
            {PAGE_SIZE_OPTIONS.map((size) => <option key={size} value={size}>{size} / page</option>)}
          </select>
          <button type="button" onClick={() => goToPage(page - 1)} disabled={!hasPrevPage || loading} aria-label="Previous page">Previous</button>
          {totalPages !== null && totalPages <= 7 && Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
            <button key={p} type="button" onClick={() => goToPage(p)} disabled={loading}
              aria-current={p === page ? "page" : undefined} className={p === page ? "active" : ""} aria-label={`Page ${p}`}>{p}</button>
          ))}
          <button type="button" onClick={() => goToPage(page + 1)} disabled={!hasNextPage || loading} aria-label="Next page">Next</button>
        </div>
      )}

      {selected && <TransactionDetails transaction={selected} onClose={() => setSelected(null)} />}
    </main>
  );
}

export default TransactionHistory;
