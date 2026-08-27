// #105 ErrorBoundary  #112 TransactionHistory route  #116 nav landmarks  #286 Onboarding
import React, { useState, useEffect } from "react";
import { BrowserRouter as Router, Routes, Route, Link } from "react-router-dom";
import RemittanceForm from "./components/RemittanceForm";
import InvoiceViewer from "./components/InvoiceViewer";
import ErrorBoundary from "./components/ErrorBoundary";
import TransactionHistory from "./pages/TransactionHistory";
import Onboarding from "./components/Onboarding";
import "./App.css";

function App() {
  const [showOnboarding, setShowOnboarding] = useState(false);

  useEffect(() => {
    const completed = localStorage.getItem("gpay_onboarding_completed");
    if (!completed) {
      setShowOnboarding(true);
    }
  }, []);

  return (
    <Router>
      <div className="App">
        <header className="App-header">
          <div className="header-brand-row">
            <h1>Gpay-Remit</h1>
            <button
              type="button"
              className="tour-nav-btn"
              onClick={() => setShowOnboarding(true)}
              aria-label="Start interactive product tour"
            >
              ✨ Product Tour
            </button>
          </div>
          {/* #116 — nav landmark so screen readers can jump straight to navigation */}
          <nav aria-label="Main navigation">
            <Link to="/">Send Remittance</Link>
            <Link to="/invoices">View Invoices</Link>
            <Link to="/transactions">Transaction History</Link>
          </nav>
        </header>

        {/* #286 — Interactive Multi-Step User Onboarding Tour */}
        <Onboarding
          isOpen={showOnboarding}
          onClose={() => setShowOnboarding(false)}
          onComplete={() => setShowOnboarding(false)}
        />

        {/* #105 — wrap route tree so any page-level render error shows a
            recoverable fallback instead of a blank screen. */}
        <ErrorBoundary>
          <main id="main-content" aria-label="Page content">
            <Routes>
              <Route path="/" element={<RemittanceForm />} />
              <Route path="/invoices" element={<InvoiceViewer />} />
              <Route path="/transactions" element={<TransactionHistory />} />
            </Routes>
          </main>
        </ErrorBoundary>
      </div>
    </Router>
  );
}

export default App;
