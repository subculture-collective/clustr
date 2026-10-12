import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';

type ErrorBoundaryProps = {
  children: ReactNode;
  fallback?: (error: Error, retry: () => void) => ReactNode;
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
  resetKeys?: Array<string | number>;
};

type ErrorBoundaryState = {
  hasError: boolean;
  error: Error | null;
  retryKey: number;
};

/**
 * ErrorBoundary - Catches JavaScript errors anywhere in the child component tree
 * Displays a fallback UI instead of crashing the entire app
 */
class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null, retryKey: 0 };
  }

  static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
    // Update state so the next render will show the fallback UI
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // Log error to console and call optional error handler
    console.error('ErrorBoundary caught an error:', error, errorInfo);
    this.props.onError?.(error, errorInfo);
  }

  componentDidUpdate(prevProps: ErrorBoundaryProps) {
    // Reset error state if resetKeys changed
    if (
      this.state.hasError &&
      this.props.resetKeys &&
      prevProps.resetKeys &&
      this.props.resetKeys.some((key, i) => key !== prevProps.resetKeys?.[i])
    ) {
      this.setState({ hasError: false, error: null });
    }
  }

  handleReset = () => {
    // Increment retryKey to force remount of children
    this.setState((state) => ({
      hasError: false,
      error: null,
      retryKey: state.retryKey + 1,
    }));
  };

  render() {
    if (this.state.hasError && this.state.error) {
      // Use custom fallback if provided, otherwise use default
      if (this.props.fallback) {
        return this.props.fallback(this.state.error, this.handleReset);
      }

      // Default fallback UI
      return (
        <div className="w-full h-screen bg-black flex items-center justify-center">
          <div className="bg-red-900/20 border border-red-500/50 rounded-lg px-8 py-6 max-w-2xl mx-4">
            <div className="flex items-start gap-4">
              {/* Error icon */}
              <div className="shrink-0">
                <svg
                  className="w-8 h-8 text-red-500"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                  />
                </svg>
              </div>

              {/* Error content */}
              <div className="flex-1">
                <h2 className="text-red-100 text-xl font-semibold mb-2">
                  Something went wrong
                </h2>
                <p className="text-red-200 text-sm mb-4">
                  The visualization component encountered an error and couldn't render properly.
                </p>
                
                {/* Error details (collapsed by default) */}
                <details className="mb-4">
                  <summary className="text-red-300 text-sm cursor-pointer hover:text-red-200">
                    Show error details
                  </summary>
                  <pre className="mt-2 text-xs text-red-200 bg-black/30 rounded-sm p-3 overflow-auto max-h-40">
                    {this.state.error.toString()}
                    {this.state.error.stack && `\n\n${this.state.error.stack}`}
                  </pre>
                </details>

                {/* Action buttons */}
                <div className="flex gap-3">
                  <button
                    onClick={this.handleReset}
                    className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-sm font-medium transition-colors"
                  >
                    Try Again
                  </button>
                  <button
                    onClick={() => window.location.reload()}
                    className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-sm font-medium transition-colors"
                  >
                    Reload Page
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      );
    }

    // Use retryKey to force remount on retry
    return <div key={this.state.retryKey}>{this.props.children}</div>;
  }
}

export default ErrorBoundary;
