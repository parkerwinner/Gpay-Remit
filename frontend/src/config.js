// #114 — centralise env-var access and validate at startup so a missing /
// misconfigured REACT_APP_API_URL surfaces as a clear console warning rather
// than a silent 404 on every API call.

const rawUrl = process.env.REACT_APP_API_URL;

function isValidUrl(str) {
  try {
    const u = new URL(str);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

const FALLBACK_URL = "http://localhost:8080/api/v1";

let API_BASE_URL;
if (!rawUrl) {
  console.warn(
    "[Gpay-Remit] REACT_APP_API_URL is not set — falling back to",
    FALLBACK_URL
  );
  API_BASE_URL = FALLBACK_URL;
} else if (!isValidUrl(rawUrl)) {
  console.warn(
    "[Gpay-Remit] REACT_APP_API_URL is not a valid URL (" + rawUrl + ") — falling back to",
    FALLBACK_URL
  );
  API_BASE_URL = FALLBACK_URL;
} else {
  API_BASE_URL = rawUrl;
}

export { API_BASE_URL };
