import { render, screen } from "@testing-library/react";
import App from "./App";

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

