// #105 — catches render errors in the component tree and shows a friendly
// fallback instead of a blank white screen.
import React from "react";

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, message: null };
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, message: error?.message || "Unknown error" };
  }

  componentDidCatch(error, info) {
    console.error("[ErrorBoundary]", error, info.componentStack);
  }

  handleReset = () => {
    this.setState({ hasError: false, message: null });
  };

  render() {
    if (this.state.hasError) {
      return (
        <div role="alert" className="error-boundary" aria-live="assertive">
          <h2>Something went wrong</h2>
          <p>{this.state.message}</p>
          <button onClick={this.handleReset} aria-label="Try again">
            Try again
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

export default ErrorBoundary;
