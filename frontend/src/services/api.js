// #114 — API_BASE_URL now comes from config.js which validates the env var.
import axios from "axios";
import { API_BASE_URL } from "../config";

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
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

export const sendRemittance = (data) => api.post("/remittances", data);
export const getRemittance = (id) => api.get(`/remittances/${id}`);
export const getRemittances = (page = 1, pageSize = 20) =>
  api.get(`/remittances?page=${page}&page_size=${pageSize}`);

export const createInvoice = (data) => api.post("/invoices", data);
export const getInvoice = (id) => api.get(`/invoices/${id}`);
export const getInvoices = () => api.get("/invoices");

export const getExchangeRate = (from, to) =>
  api.get(`/exchange-rates?from=${from}&to=${to}`);

// ── Auth & 2FA ──────────────────────────────────────────────────────────
// Uses pquerna/otp TOTP on backend; login accepts optional totp_code.
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
export const disableMFA = (password) => api.post("/auth/mfa/disable", { password, email: "" });
export const getMFAStatus = () => api.get("/auth/mfa/status");

export default api;
