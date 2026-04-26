// #114 — API_BASE_URL now comes from config.js which validates the env var.
import axios from "axios";
import { API_BASE_URL } from "../config";

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

export const sendRemittance = (data) => api.post("/remittances", data);
export const getRemittance = (id) => api.get(`/remittances/${id}`);
export const getRemittances = () => api.get("/remittances");

export const createInvoice = (data) => api.post("/invoices", data);
export const getInvoice = (id) => api.get(`/invoices/${id}`);
export const getInvoices = () => api.get("/invoices");

// #118 — exchange rate lookup. Falls back to a public open API when the
// backend endpoint is not yet wired; replace with a key-gated endpoint in prod.
export const getExchangeRate = (from, to) =>
  api.get(`/exchange-rates?from=${from}&to=${to}`).catch(() =>
    axios
      .get(`https://open.er-api.com/v6/latest/${from}`)
      .then((r) => ({ data: { rate: r.data.rates[to] } }))
  );

export default api;
