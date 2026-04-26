// #113 — generic polling hook used by InvoiceViewer for real-time status updates.
import { useEffect, useRef } from "react";

/**
 * Calls `callback` immediately then every `intervalMs` milliseconds.
 * Stops when the component unmounts or when `enabled` is false.
 */
export function usePolling(callback, intervalMs = 5000, enabled = true) {
  const savedCallback = useRef(callback);

  useEffect(() => {
    savedCallback.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!enabled) return;
    savedCallback.current();
    const id = setInterval(() => savedCallback.current(), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs, enabled]);
}
