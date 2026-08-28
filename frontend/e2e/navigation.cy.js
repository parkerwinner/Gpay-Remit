describe("Navigation and Layout", () => {
  beforeEach(() => {
    cy.visit("/");
  });

  it("should display the Gpay-Remit brand", () => {
    cy.get("h1").should("contain", "Gpay-Remit");
  });

  it("should have navigation links", () => {
    cy.get('nav[aria-label="Main navigation"]').should("exist");
    cy.get('nav a').should("have.length", 3);
    cy.get('nav a').first().should("contain", "Send Remittance");
    cy.get('nav a').eq(1).should("contain", "View Invoices");
    cy.get('nav a').eq(2).should("contain", "Transaction History");
  });

  it("should navigate to invoices page", () => {
    cy.get('nav a').contains("View Invoices").click();
    cy.url().should("include", "/invoices");
    cy.get("#invoices-heading").should("be.visible");
  });

  it("should navigate to transaction history page", () => {
    cy.get('nav a').contains("Transaction History").click();
    cy.url().should("include", "/transactions");
    cy.get("#tx-history-heading").should("be.visible");
  });

  it("should navigate back to home page", () => {
    cy.get('nav a').contains("View Invoices").click();
    cy.url().should("include", "/invoices");
    cy.get('nav a').contains("Send Remittance").click();
    cy.url().should("not.include", "/invoices");
    cy.get("#remittance-heading").should("be.visible");
  });

  it("should have skip to main content link", () => {
    cy.get(".skip-link").should("exist");
    cy.get(".skip-link").should("contain", "Skip to main content");
  });

  it("should have dark mode toggle", () => {
    cy.get(".theme-toggle").should("exist");
    cy.get(".theme-toggle").click();
    cy.get(".App").should("have.class", "dark-mode");
  });

  it("should have language selector", () => {
    cy.get('select[aria-label="Select Language"]').should("exist");
    cy.get('select[aria-label="Select Language"]').select("es");
    cy.get("h1").should("contain", "Gpay-Remit");
  });

  it("should have product tour button", () => {
    cy.get('button[aria-label="Start interactive product tour"]').should("exist");
  });

  it("should have main content area with proper aria attributes", () => {
    cy.get("main#main-content").should("exist");
    cy.get("main#main-content").should("have.attr", "aria-label", "Page content");
  });
});
