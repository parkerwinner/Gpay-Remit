import axios from "axios";

const API_BASE_URL =
  process.env.REACT_APP_API_URL || "http://localhost:8080/api/v1";

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

export const sendRemittance = (data) => {
  return api.post("/remittances", data);
};

export const getRemittance = (id) => {
  return api.get(`/remittances/${id}`);
};

export const getRemittances = () => {
  return api.get("/remittances");
};

export const createInvoice = (data) => {
  return api.post("/invoices", data);
};

export const getInvoice = (id) => {
  return api.get(`/invoices/${id}`);
};

export const getInvoices = () => {
  return api.get("/invoices");
};

export default api;
