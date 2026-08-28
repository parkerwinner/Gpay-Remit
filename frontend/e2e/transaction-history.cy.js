describe("Transaction History", () => {
  beforeEach(() => {
    cy.visit("/transactions");
  });

  it("should display the transaction history heading", () => {
    cy.get("#tx-history-heading").should("contain", "Transaction History");
  });

  it("should show filter and search controls", () => {
    cy.get("#tx-search").should("exist");
    cy.get("#tx-status").should("exist");
    cy.get("#tx-page-size").should("exist");
  });

  it("should display transactions table when data is loaded", () => {
    cy.intercept("GET", "/api/v1/remittances*", {
      statusCode: 200,
      body: [
        { id: 1, sender_id: 1, recipient_id: 2, amount: 100, currency: "USD", status: "completed", created_at: "2024-01-15" },
        { id: 2, sender_id: 3, recipient_id: 4, amount: 250, currency: "EUR", status: "pending", created_at: "2024-01-16" },
      ],
    }).as("getRemittances");

    cy.wait("@getRemittances");

    cy.get("table").should("exist");
    cy.get("table tbody tr").should("have.length", 2);
    cy.get("table tbody tr").first().should("contain", "1");
    cy.get("table tbody tr").first().should("contain", "completed");
  });

  it("should show empty state when no transactions exist", () => {
    cy.intercept("GET", "/api/v1/remittances*", {
      statusCode: 200,
      body: [],
    }).as("getRemittancesEmpty");

    cy.wait("@getRemittancesEmpty");

    cy.contains("No transactions found.").should("be.visible");
  });

  it("should filter transactions by status", () => {
    cy.intercept("GET", "/api/v1/remittances*", {
      statusCode: 200,
      body: [
        { id: 1, sender_id: 1, recipient_id: 2, amount: 100, currency: "USD", status: "completed", created_at: "2024-01-15" },
        { id: 2, sender_id: 3, recipient_id: 4, amount: 250, currency: "EUR", status: "pending", created_at: "2024-01-16" },
      ],
    }).as("getRemittances");

    cy.wait("@getRemittances");

    cy.get("#tx-status").select("completed");
    cy.get("table tbody tr").should("have.length", 1);
    cy.get("table tbody tr").first().should("contain", "completed");
  });

  it("should search transactions by ID", () => {
    cy.intercept("GET", "/api/v1/remittances*", {
      statusCode: 200,
      body: [
        { id: 1, sender_id: 1, recipient_id: 2, amount: 100, currency: "USD", status: "completed", created_at: "2024-01-15" },
        { id: 2, sender_id: 3, recipient_id: 4, amount: 250, currency: "EUR", status: "pending", created_at: "2024-01-16" },
      ],
    }).as("getRemittances");

    cy.wait("@getRemittances");

    cy.get("#tx-search").type("1");
    cy.get("table tbody tr").should("have.length", 1);
    cy.get("table tbody tr").first().should("contain", "1");
  });

  it("should open transaction details modal", () => {
    cy.intercept("GET", "/api/v1/remittances*", {
      statusCode: 200,
      body: [
        { id: 1, sender_id: 1, recipient_id: 2, amount: 100, currency: "USD", status: "completed", created_at: "2024-01-15" },
      ],
    }).as("getRemittances");

    cy.wait("@getRemittances");

    cy.get("table tbody tr").first().find("button").contains("Details").click();
    cy.get('[role="dialog"]').should("be.visible");
  });

  it("should have pagination controls", () => {
    cy.get(".tx-pagination").should("exist");
    cy.get('button[aria-label="Previous page"]').should("exist");
    cy.get('button[aria-label="Next page"]').should("exist");
    cy.get("#tx-page-size").should("exist");
  });

  it("should have proper accessibility attributes", () => {
    cy.get('[role="search"]').should("have.attr", "aria-label", "Filter transactions");
    cy.get(".tx-pagination").should("have.attr", "aria-label", "Transaction pagination");
  });
});
