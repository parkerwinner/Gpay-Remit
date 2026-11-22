/**
 * Shared mock data factories for frontend tests (#422).
 * Generates realistic fixtures for users, remittances, invoices, contacts, and webhooks.
 */

let fakerModule = null;
try {
  fakerModule = require("@faker-js/faker")?.faker;
} catch {
  fakerModule = null;
}

let counter = 1000;
const nextId = () => ++counter;

export const createMockUser = (overrides = {}) => {
  const id = overrides.id || `user_${nextId()}`;
  const firstName = fakerModule ? fakerModule.person.firstName() : "Jane";
  const lastName = fakerModule ? fakerModule.person.lastName() : "Doe";
  const email = overrides.email || (fakerModule ? fakerModule.internet.email({ firstName, lastName }) : `user${nextId()}@example.com`);

  return {
    id,
    first_name: firstName,
    last_name: lastName,
    email,
    stellar_address: overrides.stellar_address || `G${Array.from({ length: 55 }, () => "A").join("")}`,
    kyc_status: "verified",
    created_at: new Date().toISOString(),
    ...overrides,
  };
};

export const createMockRemittance = (overrides = {}) => {
  const id = overrides.id || `rem_${nextId()}`;
  const amount = overrides.amount || "250.00";
  const currency = overrides.currency || "USD";
  const targetCurrency = overrides.target_currency || "EUR";

  return {
    id,
    sender_id: overrides.sender_id || `user_${nextId()}`,
    recipient_id: overrides.recipient_id || `user_${nextId()}`,
    recipient_account: overrides.recipient_account || `G${Array.from({ length: 55 }, () => "B").join("")}`,
    amount,
    currency,
    target_currency: targetCurrency,
    exchange_rate: overrides.exchange_rate || 0.92,
    fee: overrides.fee || "2.50",
    status: overrides.status || "completed",
    tx_hash: overrides.tx_hash || `tx_${nextId()}_stellar_hash`,
    created_at: new Date().toISOString(),
    ...overrides,
  };
};

export const createMockInvoice = (overrides = {}) => {
  const id = overrides.id || `inv_${nextId()}`;
  const invoiceNumber = overrides.invoice_number || `INV-${nextId()}`;
  const amount = overrides.amount || 50000; // in minor units ($500.00)
  const currency = overrides.currency || "USD";

  return {
    id,
    invoice_number: invoiceNumber,
    issuer_id: overrides.issuer_id || `user_${nextId()}`,
    recipient_id: overrides.recipient_id || `user_${nextId()}`,
    amount,
    currency,
    status: overrides.status || "unpaid",
    due_date: overrides.due_date || new Date(Date.now() + 86400000 * 14).toISOString().split("T")[0],
    description: overrides.description || "Consulting Services",
    created_at: new Date().toISOString(),
    ...overrides,
  };
};

export const createMockContact = (overrides = {}) => {
  const id = overrides.id || `contact_${nextId()}`;
  return {
    id,
    user_id: overrides.user_id || `user_${nextId()}`,
    name: overrides.name || (fakerModule ? fakerModule.person.fullName() : "Alice Smith"),
    stellar_address: overrides.stellar_address || `G${Array.from({ length: 55 }, () => "C").join("")}`,
    created_at: new Date().toISOString(),
    ...overrides,
  };
};

export const createMockWebhook = (overrides = {}) => {
  const id = overrides.id || nextId();
  return {
    id,
    url: overrides.url || "https://consumer.example.com/webhooks",
    events: overrides.events || "payment.completed,escrow.created",
    is_active: overrides.is_active !== undefined ? overrides.is_active : true,
    secret: overrides.secret || "whsec_test_secret_key_12345",
    created_at: new Date().toISOString(),
    ...overrides,
  };
};

export const createMockExchangeRate = (overrides = {}) => {
  return {
    from: overrides.from || "USD",
    to: overrides.to || "EUR",
    rate: overrides.rate || 0.92,
    source: overrides.source || "oracle",
    timestamp: new Date().toISOString(),
    ...overrides,
  };
};
