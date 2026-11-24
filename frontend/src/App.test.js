import { render, screen, fireEvent, act } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import App from "./App";
import { LIGHT_TOKENS, DARK_TOKENS, THEME_CONTRAST_PAIRS } from "./theme/tokens";
import RemittanceForm from "./components/RemittanceForm";
import PaymentTemplates from "./components/PaymentTemplates";
import RecurringSchedules from "./components/RecurringSchedules";
import WalletDashboard from "./components/WalletDashboard";
import InvoiceViewer from "./components/InvoiceViewer";
import TransactionHistory from "./pages/TransactionHistory";
import Onboarding from "./components/Onboarding";
import ErrorBoundary from "./components/ErrorBoundary";
import { WalletProvider } from "./contexts/WalletContext";

function parseHex(hex) {
  const clean = hex.replace("#", "");
  const full = clean.length === 3 ? clean.split("").map((c) => c + c).join("") : clean;
  return [
    parseInt(full.slice(0, 2), 16) / 255,
    parseInt(full.slice(2, 4), 16) / 255,
    parseInt(full.slice(4, 6), 16) / 255,
  ];
}

function relativeLuminance(hex) {
  const [r, g, b] = parseHex(hex).map((c) =>
    c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  );
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrastRatio(hexA, hexB) {
  const l1 = relativeLuminance(hexA);
  const l2 = relativeLuminance(hexB);
  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);
  return (lighter + 0.05) / (darker + 0.05);
}

describe("App Theming & Core Rendering (#458)", () => {
  beforeEach(() => {
    localStorage.clear();
    document.body.className = "";
    document.documentElement.className = "";
    document.documentElement.removeAttribute("data-theme");
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  test("renders Gpay-Remit header", () => {
    render(<App />);
    const headerElement = screen.getByRole("heading", { level: 1, name: /Gpay-Remit/i });
    expect(headerElement).toBeInTheDocument();
  });

  test("renders product tour trigger button", () => {
    render(<App />);
    const tourBtn = screen.getByRole("button", { name: /Start interactive product tour/i });
    expect(tourBtn).toBeInTheDocument();
  });

  test("toggles dark mode classes, aria-pressed, and announcement on root element (#458)", () => {
    render(<App />);
    const toggleBtn = screen.getByRole("button", { name: /switch to dark mode/i });
    expect(toggleBtn).toHaveAttribute("aria-pressed", "false");

    fireEvent.click(toggleBtn);
    expect(document.body.classList.contains("dark")).toBe(true);
    expect(document.body.classList.contains("dark-mode")).toBe(true);
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(toggleBtn).toHaveAttribute("aria-pressed", "true");
    expect(toggleBtn).toHaveAccessibleName(/switch to light mode/i);
    expect(screen.getByText(/dark mode enabled/i)).toBeInTheDocument();

    fireEvent.click(toggleBtn);
    expect(document.body.classList.contains("dark")).toBe(false);
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(toggleBtn).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText(/light mode enabled/i)).toBeInTheDocument();
  });

  test("persists dark mode preference in localStorage and applies stored preference on load", () => {
    localStorage.setItem("gpay_dark_mode", "true");
    const { unmount } = render(<App />);
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    unmount();

    localStorage.clear();
    render(<App />);
    const toggleBtn = screen.getByRole("button", { name: /switch to dark mode/i });
    fireEvent.click(toggleBtn);
    expect(localStorage.getItem("gpay_dark_mode")).toBe("true");
    fireEvent.click(toggleBtn);
    expect(localStorage.getItem("gpay_dark_mode")).toBe("false");
  });

  test("follows prefers-color-scheme when no stored choice exists and reacts to changes", () => {
    let changeListener = null;
    window.matchMedia = jest.fn().mockImplementation((query) => ({
      matches: query === "(prefers-color-scheme: dark)",
      media: query,
      addEventListener: (_event, cb) => {
        changeListener = cb;
      },
      removeEventListener: jest.fn(),
    }));

    render(<App />);
    expect(document.documentElement.classList.contains("dark")).toBe(true);

    act(() => {
      changeListener({ matches: false });
    });
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    // Once the user explicitly toggles, system changes no longer override
    const toggleBtn = screen.getByRole("button", { name: /switch to dark mode/i });
    fireEvent.click(toggleBtn);
    expect(document.documentElement.classList.contains("dark")).toBe(true);

    act(() => {
      changeListener({ matches: false });
    });
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  test("handles localStorage failures gracefully without crashing", () => {
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });

    render(<App />);
    const toggleBtn = screen.getByRole("button", { name: /switch to (dark|light) mode/i });
    expect(() => fireEvent.click(toggleBtn)).not.toThrow();
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  test("every light theme token is also defined in dark theme", () => {
    const lightKeys = Object.keys(LIGHT_TOKENS).sort();
    const darkKeys = Object.keys(DARK_TOKENS).sort();
    expect(darkKeys).toEqual(lightKeys);
  });

  test("theme token pairs meet WCAG AA contrast in both light and dark themes", () => {
    for (const pair of THEME_CONTRAST_PAIRS) {
      const lightRatio = contrastRatio(LIGHT_TOKENS[pair.fg], LIGHT_TOKENS[pair.bg]);
      expect(lightRatio).toBeGreaterThanOrEqual(pair.minRatio);

      const darkRatio = contrastRatio(DARK_TOKENS[pair.fg], DARK_TOKENS[pair.bg]);
      expect(darkRatio).toBeGreaterThanOrEqual(pair.minRatio);
    }
  });

  test.each(["light", "dark"])("renders all pages and components cleanly in %s theme", (theme) => {
    if (theme === "dark") {
      document.documentElement.classList.add("dark", "dark-mode");
      document.documentElement.setAttribute("data-theme", "dark");
    } else {
      document.documentElement.classList.remove("dark", "dark-mode");
      document.documentElement.setAttribute("data-theme", "light");
    }

    const components = [
      <RemittanceForm key="rf" />,
      <PaymentTemplates key="pt" />,
      <RecurringSchedules key="rs" />,
      <WalletDashboard key="wd" />,
      <InvoiceViewer key="iv" />,
      <TransactionHistory key="th" />,
      <Onboarding key="ob" isOpen={true} onClose={jest.fn()} onComplete={jest.fn()} />,
      <ErrorBoundary key="eb"><span>OK</span></ErrorBoundary>,
    ];

    for (const element of components) {
      const { unmount } = render(
        <MemoryRouter>
          <WalletProvider>{element}</WalletProvider>
        </MemoryRouter>
      );
      unmount();
    }
  });
});

describe("Mobile Navigation Menu (#456)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test("toggles mobile menu, updates aria-expanded, and closes on Escape with focus return", () => {
    render(<App />);
    const menuBtn = screen.getByRole("button", { name: /open navigation menu/i });
    expect(menuBtn).toHaveAttribute("aria-expanded", "false");
    expect(menuBtn).toHaveAttribute("aria-controls", "mobile-nav-menu");

    const panel = document.getElementById("mobile-nav-menu");
    expect(panel).toHaveAttribute("hidden");

    fireEvent.click(menuBtn);
    expect(menuBtn).toHaveAttribute("aria-expanded", "true");
    expect(panel).not.toHaveAttribute("hidden");

    fireEvent.keyDown(document, { key: "Escape" });
    expect(menuBtn).toHaveAttribute("aria-expanded", "false");
    expect(panel).toHaveAttribute("hidden");
    expect(document.activeElement).toBe(menuBtn);
  });

  test("closes mobile menu when a mobile navigation link is clicked", () => {
    render(<App />);
    const menuBtn = screen.getByRole("button", { name: /open navigation menu/i });
    fireEvent.click(menuBtn);
    expect(menuBtn).toHaveAttribute("aria-expanded", "true");

    const mobileNav = screen.getByRole("navigation", { name: /mobile navigation/i });
    const historyLink = mobileNav.querySelector('a[href="/transactions"]');
    fireEvent.click(historyLink);
    expect(menuBtn).toHaveAttribute("aria-expanded", "false");
  });
});
