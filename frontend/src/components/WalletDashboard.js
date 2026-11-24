import React, { useState, useEffect, useCallback } from "react";
import { getWalletBalances, updateWalletPreferences } from "../services/api";

function formatBalance(amountStr, currency) {
  const num = Number(amountStr);
  if (isNaN(num)) return `${amountStr} ${currency}`;
  const isCrypto = currency === "XLM" || currency === "USDC";
  try {
    if (!isCrypto) {
      return new Intl.NumberFormat(undefined, {
        style: "currency",
        currency,
      }).format(num);
    }
  } catch {
    // Fallback for non-ISO codes
  }
  return `${new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: isCrypto ? 7 : 2,
  }).format(num)} ${currency}`;
}

function WalletDashboard() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [savingPref, setSavingPref] = useState(false);

  const loadWallet = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getWalletBalances();
      setData(res.data);
    } catch (err) {
      setError(err?.message || "Failed to load wallet balances.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadWallet();
  }, [loadWallet]);

  const handlePreferenceChange = async (e) => {
    const nextCurrency = e.target.value;
    setSavingPref(true);
    setError(null);
    try {
      await updateWalletPreferences(nextCurrency);
      await loadWallet();
    } catch (err) {
      setError(err?.message || "Failed to update preferred currency.");
    } finally {
      setSavingPref(false);
    }
  };

  if (loading) {
    return (
      <section className="feature-card" aria-labelledby="wallet-heading">
        <h2 id="wallet-heading">Multi-Currency Wallet</h2>
        <p>Loading wallet balances...</p>
      </section>
    );
  }

  if (error) {
    return (
      <section className="feature-card" aria-labelledby="wallet-heading">
        <h2 id="wallet-heading">Multi-Currency Wallet</h2>
        <div role="alert" className="alert-error">
          {error}
        </div>
        <button type="button" onClick={loadWallet}>
          Retry
        </button>
      </section>
    );
  }

  const balances = data?.balances || [];
  const supported = data?.supported_currencies || [];
  const preferred = data?.preferred_currency || "USD";

  return (
    <section className="feature-card" aria-labelledby="wallet-heading">
      <div className="form-row" style={{ justifyContent: "space-between", alignItems: "center" }}>
        <h2 id="wallet-heading">Multi-Currency Wallet</h2>
        <div className="form-group" style={{ marginBottom: 0 }}>
          <label htmlFor="preferred_currency_select">Preferred Currency</label>
          <select
            id="preferred_currency_select"
            value={preferred}
            onChange={handlePreferenceChange}
            disabled={savingPref}
          >
            {supported.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </div>
      </div>

      {balances.length === 0 ? (
        <p>No balances found for your wallet.</p>
      ) : (
        <>
          <div className="confirm-box" aria-label="Estimated total">
            <strong>
              Estimated Total ({preferred}):{" "}
              {formatBalance(data.estimated_total || "0.00", preferred)}
            </strong>
            <div className="item-meta">
              Display estimate only
              {data.rate_timestamp ? ` (rate as of ${new Date(data.rate_timestamp).toUTCString()})` : ""}
              {data.has_unconverted_balances
                ? " — some balances were excluded because exchange rates were unavailable."
                : ""}
            </div>
          </div>

          <ul className="item-list">
            {balances.map((b, idx) => (
              <li key={`${b.asset_code}-${idx}`} className="item-card">
                <div>
                  <strong>{b.asset_code}</strong>
                  <div className="item-meta">{formatBalance(b.balance, b.asset_code)}</div>
                </div>
                <div className="item-meta">
                  {b.rate_available
                    ? `Est. ${formatBalance(b.estimated_value, preferred)}`
                    : "Rate unavailable"}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

export default WalletDashboard;
