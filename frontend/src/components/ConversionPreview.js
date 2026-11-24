// #118 — displays live exchange-rate preview before the user submits.
// #360 — adds error handling, stale-rate fallback, and retry mechanism.
import React from "react";
import LoadingSpinner from "./LoadingSpinner";

/**
 * ConversionPreview
 *
 * Props:
 *   amount         {string|number}  Amount entered by the user
 *   fromCurrency   {string}
 *   toCurrency     {string}
 *   rate           {number|null}    Current (fresh) exchange rate
 *   lastKnownRate  {number|null}    Last successfully fetched rate (cache fallback)
 *   loading        {boolean}
 *   error          {string|null}
 *   onRetry        {function}       Called when user clicks "Retry"
 */
function ConversionPreview({
  amount,
  fromCurrency,
  toCurrency,
  rate,
  lastKnownRate = null,
  loading,
  error,
  onRetry,
}) {
  if (!amount || !fromCurrency || !toCurrency || fromCurrency === toCurrency) return null;

  const numericAmount = parseFloat(amount);
  const displayRate = rate ?? lastKnownRate;

  return (
    <div className="conversion-preview" aria-live="polite" aria-atomic="true">
      {loading && <LoadingSpinner label="Fetching exchange rate…" size={20} />}

      {!loading && error && (
        <div className="conversion-error-block" role="alert">
          {displayRate && !isNaN(numericAmount) ? (
            // Show cached (stale) rate with a warning so the user still has a reference
            <p className="conversion-stale-rate" aria-label="Stale exchange rate">
              <span className="stale-badge" title="Rate may be outdated">⚠ Stale rate</span>{" "}
              <strong>{numericAmount.toFixed(2)} {fromCurrency}</strong>
              {" ≈ "}
              <strong>{(numericAmount * displayRate).toFixed(2)} {toCurrency}</strong>
              <span className="rate-info"> (1 {fromCurrency} = {displayRate} {toCurrency})</span>
            </p>
          ) : (
            <p className="conversion-unavailable">
              Rate unavailable — unable to fetch exchange rate for {fromCurrency} → {toCurrency}.
            </p>
          )}
          {typeof onRetry === "function" && (
            <button
              type="button"
              className="retry-btn"
              onClick={onRetry}
              aria-label="Retry fetching exchange rate"
            >
              Retry
            </button>
          )}
        </div>
      )}

      {!loading && !error && displayRate && !isNaN(numericAmount) && (
        <p>
          <strong>{numericAmount.toFixed(2)} {fromCurrency}</strong>
          {" ≈ "}
          <strong>{(numericAmount * displayRate).toFixed(2)} {toCurrency}</strong>
          <span className="rate-info"> (1 {fromCurrency} = {displayRate} {toCurrency})</span>
        </p>
      )}
    </div>
  );
}

export default ConversionPreview;
