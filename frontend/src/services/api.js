// #114 — API_BASE_URL now comes from config.js which validates the env var.
import axios from "axios";
import { API_BASE_URL } from "../config";

/**
 * Shared API contracts and TypeScript types generated from backend/handlers/openapi.yaml
 * @typedef {import("../types").Remittance} Remittance
 * @typedef {import("../types").Invoice} Invoice
 * @typedef {import("../types").User} User
 * @typedef {import("../types").Paths} ApiPaths
 */

// Global loading indicator state
let activeRequestsCount = 0;
const loadingListeners = new Set();

const notifyLoadingListeners = () => {
  const isLoading = activeRequestsCount > 0;
  loadingListeners.forEach((listener) => {
    try {
      listener(isLoading);
    } catch {
      // ignore listener errors
    }
  });
};

export const subscribeLoading = (listener) => {
  loadingListeners.add(listener);
  listener(activeRequestsCount > 0);
  return () => {
    loadingListeners.delete(listener);
  };
};

export const createCancelToken = () => {
  const controller = new AbortController();
  return {
    signal: controller.signal,
    cancel: (reason) => controller.abort(reason),
  };
};

// Default timeout: 30s for reads, 60s for writes
const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    "Content-Type": "application/json",
  },
});

api.interceptors.request.use(
  (config) => {
    activeRequestsCount++;
    notifyLoadingListeners();

    // Set 60s timeout for mutation requests if not explicitly overridden
    if (["post", "put", "patch", "delete"].includes(config.method?.toLowerCase()) && !config.timeout) {
      config.timeout = 60000;
    }
    return config;
  },
  (error) => {
    activeRequestsCount = Math.max(0, activeRequestsCount - 1);
    notifyLoadingListeners();
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => {
    activeRequestsCount = Math.max(0, activeRequestsCount - 1);
    notifyLoadingListeners();
    return response;
  },
  async (error) => {
    activeRequestsCount = Math.max(0, activeRequestsCount - 1);
    notifyLoadingListeners();

    const config = error.config;
    // Retry logic for 5xx and transient network failures (max 3 retries)
    if (config && (!config._retryCount || config._retryCount < 3)) {
      const status = error.response?.status;
      const isTransient = !error.response || (status >= 500 && status <= 599);

      if (isTransient) {
        config._retryCount = (config._retryCount || 0) + 1;
        const delay = Math.min(1000 * Math.pow(2, config._retryCount - 1), 5000);
        await new Promise((resolve) => setTimeout(resolve, delay));
        return api(config);
      }
    }

    const payload = error.response?.data;
    if (payload?.error) {
      return Promise.reject(payload.error);
    }
    if (payload?.message) {
      return Promise.reject({ code: "UNKNOWN_ERROR", message: payload.message });
    }
    return Promise.reject(error);
  }
);

export const sendRemittance = (data, config = {}) => api.post("/remittances", data, config);
export const getRemittance = (id, config = {}) => api.get(`/remittances/${id}`, config);
export const getRemittances = (page = 1, pageSize = 20, config = {}) =>
  api.get(`/remittances?page=${page}&page_size=${pageSize}`, config);

export const createInvoice = (data, config = {}) => api.post("/invoices", data, config);
export const getInvoice = (id, config = {}) => api.get(`/invoices/${id}`, config);
export const getInvoices = (config = {}) => api.get("/invoices", config);

export const getExchangeRate = (from, to, config = {}) =>
  api.get(`/exchange-rates?from=${from}&to=${to}`, config);

// ── Payment Templates ───────────────────────────────────────────────────
export const getTemplates = (page = 1, pageSize = 20, config = {}) =>
  api.get(`/templates?page=${page}&page_size=${pageSize}`, config);
export const createTemplate = (data, config = {}) => api.post("/templates", data, config);
export const updateTemplate = (id, data, config = {}) => api.put(`/templates/${id}`, data, config);
export const deleteTemplate = (id, config = {}) => api.delete(`/templates/${id}`, config);
export const sendFromTemplate = (id, config = {}) => api.post(`/templates/${id}/send`, {}, config);
export const calculateFee = (amount, config = {}) => api.get(`/fees/calculate?amount=${amount}`, config);

// ── Recurring Schedules ─────────────────────────────────────────────────
export const getRecurringSchedules = (config = {}) => api.get("/recurring-schedules", config);
export const createRecurringSchedule = (data, config = {}) => api.post("/recurring-schedules", data, config);
export const pauseRecurringSchedule = (id, config = {}) => api.post(`/recurring-schedules/${id}/pause`, {}, config);
export const resumeRecurringSchedule = (id, config = {}) => api.post(`/recurring-schedules/${id}/resume`, {}, config);
export const cancelRecurringSchedule = (id, config = {}) => api.post(`/recurring-schedules/${id}/cancel`, {}, config);

// ── Multi-Currency Wallet ───────────────────────────────────────────────
export const getWalletBalances = (config = {}) => api.get("/wallet/balances", config);
export const getWalletPreferences = (config = {}) => api.get("/wallet/preferences", config);
export const updateWalletPreferences = (preferredCurrency, config = {}) =>
  api.put("/wallet/preferences", { preferred_currency: preferredCurrency }, config);

// ── Auth & 2FA ──────────────────────────────────────────────────────────
export const register = (data) => api.post("/auth/register", data);
export const login = (data) => api.post("/auth/login", data);
export const refreshToken = (data) => api.post("/auth/refresh", data);
export const logout = () => api.post("/auth/logout");
export const forgotPassword = (email) => api.post("/auth/forgot-password", { email });
export const resetPassword = (token, newPassword) =>
  api.post("/auth/reset-password", { token, new_password: newPassword });

// MFA (requires Authorization header)
export const setupMFA = (password) => api.post("/auth/mfa/setup", { password });
export const verifyMFA = (code) => api.post("/auth/mfa/verify", { code });
export const disableMFA = (password) => api.post("/auth/mfa/disable", { password });
export const getMFAStatus = () => api.get("/auth/mfa/status");

export default api;
