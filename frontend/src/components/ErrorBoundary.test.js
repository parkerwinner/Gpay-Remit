import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import ErrorBoundary from "./ErrorBoundary";

function ProblemChild() {
  throw new Error("Test error");
}

test("ErrorBoundary catches render errors and displays fallback UI", () => {
  const consoleErrorSpy = jest.spyOn(console, "error").mockImplementation(() => {});

  render(
    <ErrorBoundary>
      <ProblemChild />
    </ErrorBoundary>
  );

  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(screen.getByText(/Something went wrong/i)).toBeInTheDocument();
  expect(screen.getByText(/Test error/i)).toBeInTheDocument();
  expect(consoleErrorSpy).toHaveBeenCalled();

  consoleErrorSpy.mockRestore();
});

test("ErrorBoundary reset button clears the error state", () => {
  let shouldThrow = true;

  function RecoveringChild() {
    if (shouldThrow) {
      shouldThrow = false;
      throw new Error("Initial failure");
    }
    return <div>Recovered content</div>;
  }

  render(
    <ErrorBoundary>
      <RecoveringChild />
    </ErrorBoundary>
  );

  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(screen.getByText(/Initial failure/i)).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: /Try again/i }));

  expect(screen.getByText(/Recovered content/i)).toBeInTheDocument();
});
