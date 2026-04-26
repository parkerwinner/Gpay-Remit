// #118 — displays live exchange-rate preview before the user submits.
import React from "react";
import LoadingSpinner from "./LoadingSpinner";

function ConversionPreview({ amount, fromCurrency, toCurrency, rate, loading, error }) {
  if (!amount || !fromCurrency || !toCurrency || fromCurrency === toCurrency) return null;

  const numericAmount = parseFloat(amount);

  return (
    <div className="conversion-preview" aria-live="polite" aria-atomic="true">
      {loading && <LoadingSpinner label="Fetching exchange rate…" size={20} />}
      {error && (
        <p className="conversion-error" role="alert">
          {error}
        </p>
      )}
      {!loading && !error && rate && !isNaN(numericAmount) && (
        <p>
          <strong>{numericAmount.toFixed(2)} {fromCurrency}</strong>
          {" ≈ "}
          <strong>{(numericAmount * rate).toFixed(2)} {toCurrency}</strong>
          <span className="rate-info"> (1 {fromCurrency} = {rate} {toCurrency})</span>
        </p>
      )}
    </div>
  );
}

export default ConversionPreview;
