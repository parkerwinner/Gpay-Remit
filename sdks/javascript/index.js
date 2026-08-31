"use strict";

/**
 * GpayRemitClient — official JavaScript client for the Gpay-Remit API.
 *
 * Usage:
 *   const { GpayRemitClient } = require("@gpay-remit/sdk");
 *   const client = new GpayRemitClient({ baseUrl: "https://api.example.com" });
 *   await client.auth.login({ email, password });
 */

class GpayRemitClient {
  /**
   * @param {object} opts
   * @param {string} opts.baseUrl  API base URL (e.g. http://localhost:8080)
   * @param {string} [opts.token]  JWT bearer token (skip to authenticate later)
   */
  constructor({ baseUrl, token } = {}) {
    if (!baseUrl) throw new Error("baseUrl is required");
    this.baseUrl = baseUrl.replace(/\/+$/, "");
    this.token = token || null;

    this.auth = new AuthClient(this);
    this.remittances = new RemittanceClient(this);
    this.invoices = new InvoiceClient(this);
    this.fees = new FeeClient(this);
    this.paymentRequests = new PaymentRequestClient(this);
    this.webhooks = new WebhookClient(this);
    this.analytics = new AnalyticsClient(this);
    this.audit = new AuditClient(this);
    this.health = new HealthClient(this);
  }

  /** Set the JWT token after login. */
  setToken(token) {
    this.token = token;
  }

  /** @private */
  async request(method, path, body) {
    const headers = { "Content-Type": "application/json" };
    if (this.token) headers["Authorization"] = `Bearer ${this.token}`;

    const opts = { method, headers };
    if (body !== undefined) opts.body = JSON.stringify(body);

    const res = await fetch(`${this.baseUrl}${path}`, opts);
    const json = await res.json().catch(() => null);

    if (!res.ok) {
      const err = new Error(json?.error || `HTTP ${res.status}`);
      err.status = res.status;
      err.body = json;
      throw err;
    }
    return json;
  }
}

// ── Auth ────────────────────────────────────────────────────────────────

class AuthClient {
  constructor(client) { this._c = client; }

  register(data) { return this._c.request("POST", "/api/v2/auth/register", data); }
  login(data) { return this._c.request("POST", "/api/v2/auth/login", data); }
  refresh(data) { return this._c.request("POST", "/api/v2/auth/refresh", data); }
  logout() { return this._c.request("POST", "/api/v2/auth/logout"); }
}

// ── Remittances ─────────────────────────────────────────────────────────

class RemittanceClient {
  constructor(client) { this._c = client; }

  create(data) { return this._c.request("POST", "/api/v2/remittances/create", data); }
  send(data) { return this._c.request("POST", "/api/v2/remittances", data); }
  get(id) { return this._c.request("GET", `/api/v2/remittances/${id}`); }
  list(params) { return this._c.request("GET", `/api/v2/remittances${toQuery(params)}`); }
  complete(id) { return this._c.request("POST", `/api/v2/remittances/${id}/complete`); }
}

// ── Invoices ────────────────────────────────────────────────────────────

class InvoiceClient {
  constructor(client) { this._c = client; }

  create(data) { return this._c.request("POST", "/api/v2/invoices", data); }
  list(params) { return this._c.request("GET", `/api/v2/invoices${toQuery(params)}`); }
  get(id) { return this._c.request("GET", `/api/v2/invoices/${id}`); }
}

// ── Fees ────────────────────────────────────────────────────────────────

class FeeClient {
  constructor(client) { this._c = client; }

  calculate(params) { return this._c.request("GET", `/api/v2/fees/calculate${toQuery(params)}`); }
  exchangeRate(params) { return this._c.request("GET", `/api/v2/exchange-rates${toQuery(params)}`); }
}

// ── Payment Requests ────────────────────────────────────────────────────

class PaymentRequestClient {
  constructor(client) { this._c = client; }

  create(data) { return this._c.request("POST", "/api/v2/payment-requests", data); }
  list() { return this._c.request("GET", "/api/v2/payment-requests"); }
  get(id) { return this._c.request("GET", `/api/v2/payment-requests/${id}`); }
  accept(id) { return this._c.request("POST", `/api/v2/payment-requests/${id}/accept`); }
  reject(id) { return this._c.request("POST", `/api/v2/payment-requests/${id}/reject`); }
  cancel(id) { return this._c.request("POST", `/api/v2/payment-requests/${id}/cancel`); }
}

// ── Webhooks ────────────────────────────────────────────────────────────

class WebhookClient {
  constructor(client) { this._c = client; }

  create(data) { return this._c.request("POST", "/api/v2/webhooks", data); }
  list() { return this._c.request("GET", "/api/v2/webhooks"); }
  get(id) { return this._c.request("GET", `/api/v2/webhooks/${id}`); }
  update(id, data) { return this._c.request("PUT", `/api/v2/webhooks/${id}`, data); }
  delete(id) { return this._c.request("DELETE", `/api/v2/webhooks/${id}`); }
  deliveries(id) { return this._c.request("GET", `/api/v2/webhooks/${id}/deliveries`); }
  retryDelivery(deliveryId) { return this._c.request("POST", `/api/v2/webhooks/deliveries/${deliveryId}/retry`); }
}

// ── Analytics ───────────────────────────────────────────────────────────

class AnalyticsClient {
  constructor(client) { this._c = client; }

  volume(params) { return this._c.request("GET", `/api/v2/analytics/volume${toQuery(params)}`); }
  fees(params) { return this._c.request("GET", `/api/v2/analytics/fees${toQuery(params)}`); }
  successRate() { return this._c.request("GET", "/api/v2/analytics/success-rate"); }
  topCorridors() { return this._c.request("GET", "/api/v2/analytics/top-corridors"); }
}

// ── Audit ───────────────────────────────────────────────────────────────

class AuditClient {
  constructor(client) { this._c = client; }

  list(params) { return this._c.request("GET", `/api/v2/audit/logs${toQuery(params)}`); }
}

// ── Health ──────────────────────────────────────────────────────────────

class HealthClient {
  constructor(client) { this._c = client; }

  check() { return this._c.request("GET", "/health"); }
  ready() { return this._c.request("GET", "/health/ready"); }
  live() { return this._c.request("GET", "/health/live"); }
}

// ── Helpers ─────────────────────────────────────────────────────────────

function toQuery(params) {
  if (!params) return "";
  const qs = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== null)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join("&");
  return qs ? `?${qs}` : "";
}

module.exports = { GpayRemitClient };
