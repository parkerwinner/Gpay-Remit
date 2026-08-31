describe("Invoice Viewer", () => {
  beforeEach(() => {
    cy.visit("/invoices");
  });

  it("should display the invoices heading", () => {
    cy.get("#invoices-heading").should("contain", "Invoices");
  });

  it("should show loading skeleton initially", () => {
    cy.get(".skeleton-loader").should("exist");
  });

  it("should display invoices table when data is loaded", () => {
    cy.intercept("GET", "/api/v1/invoices", {
      statusCode: 200,
      body: [
        { id: 1, invoice_no: "INV-001", amount: 500, currency: "USD", status: "paid", created_at: "2024-01-15" },
        { id: 2, invoice_no: "INV-002", amount: 250, currency: "EUR", status: "unpaid", created_at: "2024-01-16" },
      ],
    }).as("getInvoices");

    cy.wait("@getInvoices");

    cy.get("table").should("exist");
    cy.get("table thead th").should("have.length", 5);
    cy.get("table tbody tr").should("have.length", 2);
    cy.get("table tbody tr").first().should("contain", "INV-001");
    cy.get("table tbody tr").first().should("contain", "paid");
  });

  it("should show empty state when no invoices exist", () => {
    cy.intercept("GET", "/api/v1/invoices", {
      statusCode: 200,
      body: [],
    }).as("getInvoicesEmpty");

    cy.wait("@getInvoicesEmpty");

    cy.contains("No invoices found.").should("be.visible");
  });

  it("should display error message on API failure", () => {
    cy.intercept("GET", "/api/v1/invoices", {
      statusCode: 500,
      body: { error: "Internal server error" },
    }).as("getInvoicesError");

    cy.wait("@getInvoicesError");

    cy.get('[role="alert"]').should("contain", "Failed to fetch invoices");
  });

  it("should have proper accessibility attributes", () => {
    cy.get("table").should("have.attr", "aria-labelledby", "invoices-heading");
    cy.get("table thead th").first().should("have.attr", "scope", "col");
  });
});
