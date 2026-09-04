describe("Remittance Flow", () => {
  beforeEach(() => {
    cy.visit("/");
  });

  it("should display the remittance form", () => {
    cy.get("#remittance-heading").should("contain", "Send Remittance");
    cy.get('input[name="sender_id"]').should("exist");
    cy.get('input[name="recipient_id"]').should("exist");
    cy.get('input[name="amount"]').should("exist");
    cy.get('select[name="currency"]').should("exist");
    cy.get('select[name="target_currency"]').should("exist");
  });

  it("should validate required fields before submission", () => {
    cy.get('button[type="submit"]').contains("Send Remittance").click();
    cy.get('input[name="sender_id"]').should("have.attr", "aria-invalid", "true");
    cy.get('input[name="recipient_id"]').should("have.attr", "aria-invalid", "true");
    cy.get('input[name="amount"]').should("have.attr", "aria-invalid", "true");
  });

  it("should fill in form fields and submit", () => {
    cy.intercept("POST", "/api/v1/remittances", {
      statusCode: 201,
      body: { id: 1, status: "pending", sender_id: 1, recipient_id: 2, amount: 100, currency: "USD" },
    }).as("sendRemittance");

    cy.get('input[name="sender_id"]').type("1");
    cy.get('input[name="recipient_id"]').type("2");
    cy.get('input[name="amount"]').type("100");
    cy.get('select[name="currency"]').select("USD");
    cy.get('select[name="target_currency"]').select("EUR");
    cy.get('textarea[name="notes"]').type("Test remittance");

    cy.get('button[type="submit"]').click();

    cy.wait("@sendRemittance");

    cy.get('[role="status"]').should("contain", "Remittance Sent Successfully");
    cy.get('[role="status"]').should("contain", "Payment ID: 1");
  });

  it("should show error message on API failure", () => {
    cy.intercept("POST", "/api/v1/remittances", {
      statusCode: 400,
      body: { error: "Invalid request" },
    }).as("sendRemittanceFail");

    cy.get('input[name="sender_id"]').type("1");
    cy.get('input[name="recipient_id"]').type("2");
    cy.get('input[name="amount"]').type("100");

    cy.get('button[type="submit"]').click();

    cy.wait("@sendRemittanceFail");

    cy.get('[role="alert"]').should("contain", "Invalid request");
  });

  it("should update character count for notes field", () => {
    cy.get('textarea[name="notes"]').type("Hello");
    cy.get("#notes-hint").should("contain", "495 characters remaining");
  });

  it("should allow currency selection", () => {
    cy.get('select[name="currency"]').select("NGN");
    cy.get('select[name="currency"]').should("have.value", "NGN");
    cy.get('select[name="target_currency"]').select("KES");
    cy.get('select[name="target_currency"]').should("have.value", "KES");
  });

  it("should have proper accessibility attributes", () => {
    cy.get('input[name="sender_id"]').should("have.attr", "aria-required", "true");
    cy.get('input[name="recipient_id"]').should("have.attr", "aria-required", "true");
    cy.get('input[name="amount"]').should("have.attr", "aria-required", "true");
    cy.get('textarea[name="notes"]').should("have.attr", "aria-describedby", "notes-hint");
  });
});
