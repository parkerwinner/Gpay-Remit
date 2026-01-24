import React from "react";
import { BrowserRouter as Router, Routes, Route, Link } from "react-router-dom";
import RemittanceForm from "./components/RemittanceForm";
import InvoiceViewer from "./components/InvoiceViewer";
import "./App.css";

function App() {
  return (
    <Router>
      <div className="App">
        <header className="App-header">
          <h1>Gpay-Remit</h1>
          <nav>
            <Link to="/">Send Remittance</Link>
            <Link to="/invoices">View Invoices</Link>
          </nav>
        </header>
        <main>
          <Routes>
            <Route path="/" element={<RemittanceForm />} />
            <Route path="/invoices" element={<InvoiceViewer />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;
