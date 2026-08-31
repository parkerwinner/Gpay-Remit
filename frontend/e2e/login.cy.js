describe("Login Page", () => {
  beforeEach(() => {
    cy.visit("/");
  });

  it("should display the login form", () => {
    cy.contains("Gpay-Remit").should("be.visible");
    cy.get('input[name="email"]').should("exist");
    cy.get('input[name="password"]').should("exist");
    cy.get('button[type="submit"]').should("exist");
  });

  it("should show validation errors for empty fields", () => {
    cy.get('button[type="submit"]').click();
    cy.get('input[name="email"]').should("have.attr", "aria-invalid", "true");
    cy.get('input[name="password"]').should("have.attr", "aria-invalid", "true");
  });

  it("should show error for invalid credentials", () => {
    cy.get('input[name="email"]').type("invalid@example.com");
    cy.get('input[name="password"]').type("wrongpassword");
    cy.get('button[type="submit"]').click();
    cy.get('[role="alert"]').should("contain", "Invalid credentials");
  });
});
