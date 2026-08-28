Cypress.Commands.add("login", (email, password) => {
  cy.visit("/api/v1/auth/login");
  cy.get('input[name="email"]').type(email);
  cy.get('input[name="password"]').type(password);
  cy.get('button[type="submit"]').click();
});

Cypress.Commands.add("apiLogin", (email, password) => {
  cy.request({
    method: "POST",
    url: "/api/v1/auth/login",
    body: { email, password },
  }).then((response) => {
    window.localStorage.setItem("accessToken", response.body.access_token);
    window.localStorage.setItem("refreshToken", response.body.refresh_token);
  });
});
