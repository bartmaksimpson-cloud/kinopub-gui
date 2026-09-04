import { Component, type ErrorInfo, type ReactNode } from "react";

// ErrorBoundary keeps a render error from unmounting the whole tree. Without
// one, a single bad API response (e.g. "dirs": null reaching DirPicker) left
// the window completely black with nothing to act on — see system.go, which
// now always sends a list. This is the net under that: show what broke and
// offer a reload, rather than a blank screen.
//
// It sits outside I18nProvider on purpose, so it still renders when the
// providers themselves are what failed — hence the untranslated strings.
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Unhandled UI error:", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-ink-950 p-8 text-center">
        <h1 className="text-lg font-medium text-slate-200">Something broke in the interface</h1>
        <pre className="max-w-xl overflow-x-auto rounded-xl border border-white/[0.08] bg-ink-900/70 p-4 text-left font-mono text-xs text-ember-400">
          {error.message || String(error)}
        </pre>
        <button className="btn-ghost px-4 py-2 text-sm" onClick={() => location.reload()}>
          Reload
        </button>
      </div>
    );
  }
}
