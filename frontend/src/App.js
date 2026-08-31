// #105 ErrorBoundary  #112 TransactionHistory route  #116 nav landmarks  #286 Onboarding
import React, { useState, useEffect, createContext } from "react";
import { BrowserRouter as Router, Routes, Route, Link } from "react-router-dom";
import RemittanceForm from "./components/RemittanceForm";
import InvoiceViewer from "./components/InvoiceViewer";
import ErrorBoundary from "./components/ErrorBoundary";
import TransactionHistory from "./pages/TransactionHistory";
import Onboarding from "./components/Onboarding";
import { WalletProvider } from "./contexts/WalletContext";
import "./App.css";

// #290: Simple i18n Context
export const I18nContext = createContext();

const translations = {
  en: {
    brand: "Gpay-Remit",
    tour: "✨ Product Tour",
    send: "Send Remittance",
    invoices: "View Invoices",
    history: "Transaction History",
    toggleDark: "Toggle Dark Mode",
    skipToMain: "Skip to main content"
  },
  es: {
    brand: "Gpay-Remit",
    tour: "✨ Recorrido del producto",
    send: "Enviar Remesa",
    invoices: "Ver Facturas",
    history: "Historial de Transacciones",
    toggleDark: "Alternar modo oscuro",
    skipToMain: "Saltar al contenido principal"
  }
};

function App() {
  const [showOnboarding, setShowOnboarding] = useState(false);
  // #288: Dark Mode State
  const [darkMode, setDarkMode] = useState(false);
  // #290: Language State
  const [lang, setLang] = useState("en");

  useEffect(() => {
    const completed = localStorage.getItem("gpay_onboarding_completed");
    if (!completed) {
      setShowOnboarding(true);
    }
  }, []);

  const t = translations[lang] || translations.en;

  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>
      <WalletProvider>
        <Router>
          <div className={`App ${darkMode ? 'dark-mode' : ''}`}>
            {/* #291: a11y Skip link */}
            <a href="#main-content" className="skip-link">
              {t.skipToMain}
            </a>
            <header className="App-header">
              <div className="header-brand-row">
                <h1>{t.brand}</h1>
                <div className="header-controls">
                  <select value={lang} onChange={(e) => setLang(e.target.value)} aria-label="Select Language">
                    <option value="en">EN</option>
                    <option value="es">ES</option>
                  </select>
                  <button 
                    onClick={() => setDarkMode(!darkMode)} 
                    aria-label={t.toggleDark}
                    className="theme-toggle"
                  >
                    {darkMode ? '☀️' : '🌙'}
                  </button>
                  <button
                    type="button"
                    className="tour-nav-btn"
                    onClick={() => setShowOnboarding(true)}
                    aria-label="Start interactive product tour"
                  >
                    {t.tour}
                  </button>
                </div>
              </div>
              {/* #116 — nav landmark so screen readers can jump straight to navigation */}
              <nav aria-label="Main navigation">
                <Link to="/">{t.send}</Link>
                <Link to="/invoices">{t.invoices}</Link>
                <Link to="/transactions">{t.history}</Link>
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
              <main id="main-content" aria-label="Page content" tabIndex="-1">
                <Routes>
                  <Route path="/" element={<RemittanceForm />} />
                  <Route path="/invoices" element={<InvoiceViewer />} />
                  <Route path="/transactions" element={<TransactionHistory />} />
                </Routes>
              </main>
            </ErrorBoundary>
          </div>
        </Router>
      </WalletProvider>
    </I18nContext.Provider>
  );
}

export default App;
