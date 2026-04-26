// #117 — lightweight XSS-safe output encoding without a heavy lib dependency.
// For rich HTML content use DOMPurify; for plain-text display these helpers
// are sufficient and keep the bundle small.

/**
 * Escape HTML special characters before inserting text into the DOM.
 * Use when you must render a string as innerHTML (prefer textContent instead).
 */
export function escapeHtml(str) {
  if (typeof str !== "string") return "";
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#x27;");
}

/**
 * Strip all HTML tags from a string, returning plain text.
 * Safe for values that will be rendered via React (which auto-escapes).
 */
export function stripTags(str) {
  if (typeof str !== "string") return "";
  return str.replace(/<[^>]*>/g, "");
}

/**
 * Sanitize a user-supplied string for safe display:
 * strips tags then escapes remaining special chars.
 */
export function sanitizeInput(str) {
  return escapeHtml(stripTags(str));
}
