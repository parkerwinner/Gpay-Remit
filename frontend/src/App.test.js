import { render, screen, fireEvent } from "@testing-library/react";
import App from "./App";

describe("App Theming & Core Rendering (#458)", () => {
  beforeEach(() => {
    localStorage.clear();
    document.body.className = "";
    document.documentElement.className = "";
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

  test("toggles dark mode classes on root element and body (#458)", () => {
    render(<App />);
    const toggleBtn = screen.getByRole("button", { name: /toggle dark mode|modo oscuro/i });
    expect(toggleBtn).toBeInTheDocument();

    // Toggle to dark mode
    fireEvent.click(toggleBtn);
    expect(document.body.classList.contains("dark")).toBe(true);
    expect(document.body.classList.contains("dark-mode")).toBe(true);
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");

    // Toggle back to light mode
    fireEvent.click(toggleBtn);
    expect(document.body.classList.contains("dark")).toBe(false);
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  test("persists dark mode preference in localStorage", () => {
    render(<App />);
    const toggleBtn = screen.getByRole("button", { name: /toggle dark mode|modo oscuro/i });

    fireEvent.click(toggleBtn);
    expect(localStorage.getItem("gpay_dark_mode")).toBe("true");

    fireEvent.click(toggleBtn);
    expect(localStorage.getItem("gpay_dark_mode")).toBe("false");
  });
});
