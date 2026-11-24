// #456: Responsive viewport & mobile navigation E2E specs.
// Stubs all API calls via cy.intercept; never touches a real backend or Stellar network.

const VIEWPORTS = [
  { name: "mobile", preset: "iphone-x", isMobile: true },
  { name: "tablet", preset: "ipad-2", isMobile: false },
  { name: "desktop", width: 1280, height: 720, isMobile: false },
];

const ROUTES = ["/", "/templates", "/recurring", "/wallet", "/invoices", "/transactions"];

describe("Responsive Layout & Mobile Navigation (#456)", () => {
  beforeEach(() => {
    cy.intercept("GET", "**/api/v1/remittances*", {
      statusCode: 200,
      body: {
        data: [
          {
            id: 1,
            sender_id: 1,
            recipient_id: 2,
            amount: 10000,
            currency: "USD",
            status: "completed",
            created_at: "2026-01-01T00:00:00Z",
            tags: ["family"],
          },
        ],
        total: 1,
      },
    }).as("getRemittances");

    cy.intercept("POST", "**/api/v1/remittances", {
      statusCode: 201,
      body: { id: 99, status: "pending" },
    }).as("sendRemittance");

    cy.intercept("GET", "**/api/v1/templates*", { statusCode: 200, body: { data: [] } });
    cy.intercept("GET", "**/api/v1/recurring-schedules*", { statusCode: 200, body: { data: [] } });
    cy.intercept("GET", "**/api/v1/wallet/balances*", { statusCode: 200, body: { balances: [] } });
    cy.intercept("GET", "**/api/v1/wallet/preferences*", { statusCode: 200, body: { preferred_currency: "USD" } });
    cy.intercept("GET", "**/api/v1/invoices*", { statusCode: 200, body: { data: [] } });
    cy.intercept("GET", "**/api/v1/exchange-rates*", { statusCode: 200, body: { rate: 1.0 } });
    cy.intercept("GET", "**/api/v1/remittances/tags/summary*", { statusCode: 200, body: { tags: [] } });
    cy.intercept("GET", "**/api/v1/remittances/tags/autocomplete*", { statusCode: 200, body: { tags: [] } });
  });

  VIEWPORTS.forEach((vp) => {
    describe(`Viewport: ${vp.name}`, () => {
      beforeEach(() => {
        if (vp.preset) {
          cy.viewport(vp.preset);
        } else {
          cy.viewport(vp.width, vp.height);
        }
        window.localStorage.setItem("gpay_onboarding_completed", "true");
      });

      ["light", "dark"].forEach((theme) => {
        it(`has no horizontal overflow on all routes in ${theme} theme`, () => {
          window.localStorage.setItem("gpay_dark_mode", theme === "dark" ? "true" : "false");
          ROUTES.forEach((route) => {
            cy.visit(route);
            cy.window().then((win) => {
              expect(win.document.documentElement.scrollWidth).to.be.at.most(win.innerWidth);
            });
          });
        });
      });

      it("shows the appropriate navigation for the viewport", () => {
        cy.visit("/");
        if (vp.isMobile) {
          cy.get("nav.desktop-nav").should("not.be.visible");
          cy.get("button.mobile-menu-toggle").should("be.visible").click();
          cy.get("#mobile-nav-menu").should("be.visible");

          // Escape closes menu and returns focus to toggle button
          cy.get("body").type("{esc}");
          cy.get("#mobile-nav-menu").should("not.be.visible");
          cy.focused().should("have.class", "mobile-menu-toggle");

          // Re-open and navigate to Transaction History
          cy.get("button.mobile-menu-toggle").click();
          cy.get('#mobile-nav-menu a[href="/transactions"]').click();
          cy.url().should("include", "/transactions");
          cy.get("#mobile-nav-menu").should("not.be.visible");
        } else {
          cy.get("nav.desktop-nav").should("be.visible");
          cy.get("button.mobile-menu-toggle").should("not.be.visible");
        }
      });

      it("renders RemittanceForm with >= 44px touch targets, >= 16px inputs, and submits", () => {
        cy.visit("/");
        cy.get("#sender_id").should("be.visible").then(($el) => {
          expect(parseFloat(getComputedStyle($el[0]).fontSize)).to.be.at.least(16);
          expect($el[0].getBoundingClientRect().height).to.be.at.least(44);
        });
        cy.get('form button[type="submit"]').should("be.visible").then(($btn) => {
          expect($btn[0].getBoundingClientRect().height).to.be.at.least(44);
        });

        cy.get("#sender_id").type("1");
        cy.get("#recipient_id").type("2");
        cy.get("#recipient_account").type("GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H");
        cy.get("#amount").type("25");
        cy.get('form button[type="submit"]').click();
        cy.wait("@sendRemittance");
      });

      if (vp.isMobile) {
        it("renders payment tags and tag filter cleanly on mobile", () => {
          cy.visit("/transactions");
          cy.get("#tx-tag-filter").should("be.visible");
          cy.get(".tag-chip").contains("family").should("be.visible");
          cy.window().then((win) => {
            expect(win.document.documentElement.scrollWidth).to.be.at.most(win.innerWidth);
          });
        });
      }
    });
  });
});
