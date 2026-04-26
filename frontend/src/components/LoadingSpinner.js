// #109 — accessible spinner used during async operations.
import React from "react";

function LoadingSpinner({ label = "Loading…", size = 32 }) {
  return (
    <div
      role="status"
      aria-label={label}
      className="loading-spinner"
      style={{ width: size, height: size }}
    >
      <svg
        viewBox="0 0 50 50"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
        focusable="false"
      >
        <circle
          cx="25"
          cy="25"
          r="20"
          fill="none"
          stroke="currentColor"
          strokeWidth="4"
          strokeDasharray="90 60"
        >
          <animateTransform
            attributeName="transform"
            type="rotate"
            from="0 25 25"
            to="360 25 25"
            dur="0.8s"
            repeatCount="indefinite"
          />
        </circle>
      </svg>
      <span className="sr-only">{label}</span>
    </div>
  );
}

export default LoadingSpinner;
