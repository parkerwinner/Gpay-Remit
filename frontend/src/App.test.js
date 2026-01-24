import { render, screen } from "@testing-library/react";
import App from "./App";

test("renders Gpay-Remit header", () => {
  render(<App />);
  const headerElement = screen.getByText(/Gpay-Remit/i);
  expect(headerElement).toBeInTheDocument();
});
