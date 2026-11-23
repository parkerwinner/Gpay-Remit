// #105 ErrorBoundary  #112 TransactionHistory route  #116 nav landmarks  #286 Onboarding
import React, { useState, useEffect, createContext } from "react";
import { BrowserRouter as Router, Routes, Route, Link } from "react-router-dom";
import RemittanceForm from "./components/RemittanceForm";
import PaymentTemplates from "./components/PaymentTemplates";
import RecurringSchedules from "./components/RecurringSchedules";
import WalletDashboard from "./components/WalletDashboard";
import InvoiceViewer from "./components/InvoiceViewer";
import ErrorBoundary from "./components/ErrorBoundary";
import TransactionHistory from "./pages/TransactionHistory";
import Onboarding from "./components/Onboarding";
import { WalletProvider } from "./contexts/WalletContext";
import { translations, getTranslation } from "./locales/i18n";
import "./App.css";

// #290: Simple i18n Context
export const I18nContext = createContext();

function detectBrowserLanguage() {
  try {
    if (typeof navigator !== "undefined" && navigator.language) {
      const code = navigator.language.split("-")[0].toLowerCase();
      if (["en", "es", "pt", "fr"].includes(code)) {
        return code;
      }
    }
  } catch {
    // fallback
  }
  return "en";
}

function App() {
  const [showOnboarding, setShowOnboarding] = useState(false);
  // #288: Dark Mode State
  // Initial value: saved preference, else the OS colour-scheme preference.
  const [darkMode, setDarkMode] = useState(() => {
    try {
      const stored = localStorage.getItem("gpay_dark_mode");
      if (stored === "true" || stored === "false") return stored === "true";
    } catch {
      // storage unavailable (private mode / blocked): fall back to OS preference
    }
    return typeof window !== "undefined" && typeof window.matchMedia === "function"
      ? window.matchMedia("(prefers-color-scheme: dark)").matches
      : false;
  });

  // Persist the preference and mirror it onto <html>, <body>, and root for global theming (#458).
  useEffect(() => {
    try {
      localStorage.setItem("gpay_dark_mode", String(darkMode));
    } catch {
      // ignore write failures
    }
    document.body.classList.toggle("dark-mode", darkMode);
    document.body.classList.toggle("dark", darkMode);
    if (typeof document !== "undefined" && document.documentElement) {
      document.documentElement.classList.toggle("dark-mode", darkMode);
      document.documentElement.classList.toggle("dark", darkMode);
      document.documentElement.setAttribute("data-theme", darkMode ? "dark" : "light");
    }
  }, [darkMode]);
  // #290: Language State with browser language detection
  const [lang, setLang] = useState(() => {
    return localStorage.getItem("gpay_lang") || detectBrowserLanguage();
  });

  useEffect(() => {
    localStorage.setItem("gpay_lang", lang);
  }, [lang]);

  useEffect(() => {
    const completed = localStorage.getItem("gpay_onboarding_completed");
    if (!completed) {
      setShowOnboarding(true);
    }
  }, []);

  // Provide fallback: lang -> en -> key
  const t = (key) => getTranslation(lang, key);
  // Attach common keys directly for backwards compatibility (e.g. t.brand)
  const currentTranslations = translations[lang] || translations.en;
  Object.keys(translations.en).forEach((key) => {
    t[key] = currentTranslations[key] || translations.en[key] || key;
  });

  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>
      <WalletProvider>
        <Router>
          <div className={`App ${darkMode ? "dark-mode dark" : ""}`} data-theme={darkMode ? "dark" : "light"}>
            {/* #291: a11y Skip link */}
            <a href="#main-content" className="skip-link">
              {t.skipToMain}
            </a>
            <header className="App-header">
              <div className="header-brand-row">
                <h1>{t.brand}</h1>
                <div className="header-controls">
                  <select
                    value={lang}
                    onChange={(e) => setLang(e.target.value)}
                    aria-label="Select Language"
                  >
                    <option value="en">EN</option>
                    <option value="es">ES</option>
                    <option value="pt">PT</option>
                    <option value="fr">FR</option>
                  </select>
                  <button
                    onClick={() => setDarkMode(!darkMode)}
                    aria-label={t.toggleDark}
                    className="theme-toggle"
                  >
                    {darkMode ? "☀️" : "🌙"}
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
                <Link to="/templates">{t.templates}</Link>
                <Link to="/recurring">{t.recurring}</Link>
                <Link to="/wallet">{t.wallet}</Link>
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
                  <Route path="/templates" element={<PaymentTemplates />} />
                  <Route path="/recurring" element={<RecurringSchedules />} />
                  <Route path="/wallet" element={<WalletDashboard />} />
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
