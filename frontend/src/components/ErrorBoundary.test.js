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
  // In React 18 the error boundary catches an error, shows the fallback, and
  // a "Try again" button resets the state. We test this by:
  //   1. Rendering with a child that always throws → boundary shows fallback.
  //   2. Clicking reset → boundary clears its error state and re-renders children.
  //   3. Re-rendering the tree with a non-throwing child → normal content shown.
  //
  // We suppress console.error because React prints uncaught errors in tests.
  const consoleErrorSpy = jest
    .spyOn(console, "error")
    .mockImplementation(() => {});

  function ThrowingChild() {
    throw new Error("Initial failure");
  }

  const { rerender } = render(
    <ErrorBoundary>
      <ThrowingChild />
    </ErrorBoundary>
  );

  // Boundary should show the error fallback.
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(screen.getByText(/Initial failure/i)).toBeInTheDocument();

  // Re-render with a safe child BEFORE clicking reset so that when the
  // boundary resets its state and re-renders, the safe child renders normally.
  function NormalChild() {
    return <div>Recovered content</div>;
  }

  rerender(
    <ErrorBoundary>
      <NormalChild />
    </ErrorBoundary>
  );

  // Now click reset — the boundary clears hasError, children re-render safely.
  fireEvent.click(screen.getByRole("button", { name: /Try again/i }));

  expect(screen.getByText(/Recovered content/i)).toBeInTheDocument();

  consoleErrorSpy.mockRestore();
});
