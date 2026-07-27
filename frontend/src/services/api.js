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

export default api;
