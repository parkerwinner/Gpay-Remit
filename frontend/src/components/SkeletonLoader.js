// #109 — generic skeleton-screen placeholder shown while data loads.
import React from "react";

function SkeletonLoader({ rows = 5, columns = 5 }) {
  return (
    <div className="skeleton-table" aria-busy="true" aria-label="Loading content">
      <table aria-hidden="true">
        <thead>
          <tr>
            {Array.from({ length: columns }).map((_, i) => (
              <th key={i}>
                <span className="skeleton-cell" />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: rows }).map((_, r) => (
            <tr key={r}>
              {Array.from({ length: columns }).map((_, c) => (
                <td key={c}>
                  <span className="skeleton-cell" />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default SkeletonLoader;
