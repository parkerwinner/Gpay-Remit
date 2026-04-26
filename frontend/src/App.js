// #105 ErrorBoundary  #112 TransactionHistory route  #116 nav landmarks
import React from "react";
import { BrowserRouter as Router, Routes, Route, Link } from "react-router-dom";
import RemittanceForm from "./components/RemittanceForm";
import InvoiceViewer from "./components/InvoiceViewer";
import ErrorBoundary from "./components/ErrorBoundary";
import TransactionHistory from "./pages/TransactionHistory";
import "./App.css";

function App() {
  return (
    <Router>
      <div className="App">
        <header className="App-header">
          <h1>Gpay-Remit</h1>
          {/* #116 — nav landmark so screen readers can jump straight to navigation */}
          <nav aria-label="Main navigation">
            <Link to="/">Send Remittance</Link>
            <Link to="/invoices">View Invoices</Link>
            <Link to="/transactions">Transaction History</Link>
          </nav>
        </header>
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
