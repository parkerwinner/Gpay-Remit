import React from "react";

const WALLET_OPTIONS = [
  {
    id: "freighter",
    label: "Freighter",
    description: "Browser extension by Stellar Development Foundation",
    icon: "🔐",
  },
  {
    id: "ledger",
    label: "Ledger",
    description: "Hardware wallet — connect via USB",
    icon: "🔑",
  },
  {
    id: "albedo",
    label: "Albedo",
    description: "Web-based Stellar wallet",
    icon: "✨",
  },
  {
    id: "xbull",
    label: "xBull",
    description: "xBull browser extension wallet",
    icon: "🐂",
  },
];

export function WalletSelectModal({ onSelect, onClose, isConnecting, error }) {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Select wallet"
      style={{
        position: "fixed",
        inset: 0,
        background: "var(--overlay-bg, rgba(0, 0, 0, 0.5))",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 1000,
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        style={{
          background: "var(--surface)",
          color: "var(--text)",
          border: "1px solid var(--border)",
          borderRadius: "12px",
          padding: "24px",
          minWidth: "280px",
          maxWidth: "400px",
          width: "100%",
        }}
      >
        <h2 style={{ margin: "0 0 16px", fontSize: "1.2rem" }}>Connect a wallet</h2>

        {error && (
          <p role="alert" style={{ color: "var(--danger-text)", marginBottom: "12px", fontSize: "0.9rem" }}>
            {error}
          </p>
        )}

        <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {WALLET_OPTIONS.map((w) => (
            <li key={w.id}>
              <button
                type="button"
                onClick={() => onSelect(w.id)}
                disabled={isConnecting}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: "12px",
                  width: "100%",
                  padding: "12px 16px",
                  marginBottom: "8px",
                  border: "1px solid var(--border)",
                  borderRadius: "8px",
                  background: isConnecting ? "var(--surface-subtle)" : "var(--surface)",
                  color: "var(--text)",
                  cursor: isConnecting ? "not-allowed" : "pointer",
                  textAlign: "left",
                  fontSize: "1rem",
                }}
              >
                <span style={{ fontSize: "1.5rem" }}>{w.icon}</span>
                <span>
                  <strong>{w.label}</strong>
                  <br />
                  <small style={{ color: "var(--muted-text)" }}>{w.description}</small>
                </span>
              </button>
            </li>
          ))}
        </ul>

        <button
          type="button"
          onClick={onClose}
          disabled={isConnecting}
          style={{
            marginTop: "8px",
            width: "100%",
            padding: "10px",
            border: "none",
            background: "transparent",
            cursor: "pointer",
            color: "var(--muted-text)",
          }}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
