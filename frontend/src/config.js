/**
 * Required environment variables:
 *   REACT_APP_API_URL          - Base URL of the Gpay-Remit backend API
 *                                e.g. http://localhost:8080/api/v1
 *   REACT_APP_STELLAR_NETWORK  - Stellar network: "testnet" or "mainnet"
 *                                (optional — defaults to "testnet")
 *
 * In development, copy frontend/.env.example to frontend/.env and fill in the values.
 */

const DEV_FALLBACK_URL = "http://localhost:8080/api/v1";
const IS_DEV = process.env.NODE_ENV === "development";

function isValidUrl(str) {
  try {
    const u = new URL(str);
    return u.protocol === "http:" || u.protocol === "https:";
  } catch {
    return false;
  }
}

function resolveApiUrl() {
  const raw = process.env.REACT_APP_API_URL;

  if (!raw || raw.trim() === "") {
    if (IS_DEV) {
      console.warn(
        `[Gpay-Remit] REACT_APP_API_URL is not set. Using development fallback: ${DEV_FALLBACK_URL}`
      );
      return DEV_FALLBACK_URL;
    }
    throw new Error(
      "[Gpay-Remit] REACT_APP_API_URL is required but not set. " +
        "Set it in your environment or .env file before starting the app."
    );
  }

  if (!isValidUrl(raw)) {
    if (IS_DEV) {
      console.warn(
        `[Gpay-Remit] REACT_APP_API_URL "${raw}" is not a valid http/https URL. ` +
          `Using development fallback: ${DEV_FALLBACK_URL}`
      );
      return DEV_FALLBACK_URL;
    }
    throw new Error(
      `[Gpay-Remit] REACT_APP_API_URL "${raw}" is not a valid http/https URL. ` +
        "Provide a full URL including protocol, e.g. https://api.example.com/api/v1"
    );
  }

  return raw.replace(/\/$/, "");
}

export const API_BASE_URL = resolveApiUrl();

export const STELLAR_NETWORK =
  process.env.REACT_APP_STELLAR_NETWORK || "testnet";
