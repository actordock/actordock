import { useState } from "react";
import { PlatformAPIError, testConnection } from "../api/platform";
import { MarbleCard, PageHeader } from "../components";
import {
  defaultPrefs,
  loadPrefs,
  notifyPrefsUpdated,
  savePrefs,
  type DashboardPrefs,
  type ThemeDensity,
} from "../settings/prefs";
import "./Settings.css";

type TestState =
  | { kind: "idle" }
  | { kind: "testing" }
  | { kind: "ok"; sandboxCount: number }
  | { kind: "error"; message: string; hints: string[] };

export function Settings() {
  const [prefs, setPrefs] = useState<DashboardPrefs>(() => loadPrefs());
  const [saved, setSaved] = useState(false);
  const [testState, setTestState] = useState<TestState>({ kind: "idle" });

  const update = <K extends keyof DashboardPrefs>(
    key: K,
    value: DashboardPrefs[K],
  ) => {
    setPrefs((prev) => ({ ...prev, [key]: value }));
    setSaved(false);
  };

  const handleSave = () => {
    savePrefs(prefs);
    notifyPrefsUpdated();
    setSaved(true);
  };

  const handleReset = () => {
    setPrefs({ ...defaultPrefs });
    setSaved(false);
  };

  const handleTest = async () => {
    setTestState({ kind: "testing" });
    try {
      const result = await testConnection();
      setTestState({
        kind: "ok",
        sandboxCount: result.sandboxes.length,
      });
    } catch (err) {
      const message =
        err instanceof PlatformAPIError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Connection failed";
      setTestState({
        kind: "error",
        message,
        hints: connectionHints(err),
      });
    }
  };

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="Platform connection and dashboard preferences."
        actions={
          <button type="button" className="btn btn--primary" onClick={handleSave}>
            Save preferences
          </button>
        }
      />

      <div className="settings-grid">
        <MarbleCard title="Connection">
          <p className="settings-note">
            Connection tests use the dashboard server proxy at{" "}
            <code className="mono">/api/platform</code>. In Kubernetes, set{" "}
            <code className="mono">ACTORDOCK_PLATFORM_URL</code> and{" "}
            <code className="mono">ACTORDOCK_API_KEY</code> on the dashboard
            Deployment.
          </p>

          <div className="settings-fields">
            <label className="settings-field">
              <span>Platform URL (optional override)</span>
              <input
                type="url"
                placeholder="http://platform:8080"
                value={prefs.platformURL}
                onChange={(e) => update("platformURL", e.target.value)}
              />
            </label>
            <label className="settings-field">
              <span>API key (optional override)</span>
              <input
                type="password"
                placeholder="Server env preferred in production"
                value={prefs.apiKey}
                onChange={(e) => update("apiKey", e.target.value)}
                autoComplete="off"
              />
            </label>
          </div>

          <div className="settings-actions">
            <button
              type="button"
              className="btn btn--primary"
              onClick={() => void handleTest()}
              disabled={testState.kind === "testing"}
            >
              {testState.kind === "testing" ? "Testing…" : "Test connection"}
            </button>
          </div>

          {testState.kind === "ok" ? (
            <div className="settings-panel settings-panel--ok" role="status">
              Platform reachable. Listed {testState.sandboxCount} sandbox
              {testState.sandboxCount === 1 ? "" : "es"}.
            </div>
          ) : null}

          {testState.kind === "error" ? (
            <div className="settings-panel settings-panel--error" role="alert">
              <strong>Could not reach Platform</strong>
              <p>{testState.message}</p>
              <ul>
                {testState.hints.map((hint) => (
                  <li key={hint}>{hint}</li>
                ))}
              </ul>
            </div>
          ) : null}
        </MarbleCard>

        <MarbleCard title="UI preferences">
          <div className="settings-fields">
            <label className="settings-field">
              <span>Auto-refresh interval</span>
              <select
                value={prefs.refreshIntervalSec}
                onChange={(e) =>
                  update("refreshIntervalSec", Number(e.target.value))
                }
              >
                <option value={15}>15 seconds</option>
                <option value={30}>30 seconds</option>
                <option value={60}>60 seconds</option>
                <option value={0}>Off</option>
              </select>
            </label>

            <label className="settings-field">
              <span>Log tail lines (logs tab)</span>
              <input
                type="number"
                min={50}
                max={5000}
                step={50}
                value={prefs.logTailLines}
                onChange={(e) =>
                  update("logTailLines", Number(e.target.value) || 200)
                }
              />
            </label>

            <label className="settings-field">
              <span>Theme density</span>
              <select
                value={prefs.themeDensity}
                onChange={(e) =>
                  update("themeDensity", e.target.value as ThemeDensity)
                }
              >
                <option value="comfortable">Comfortable</option>
                <option value="compact">Compact</option>
              </select>
            </label>
          </div>

          <div className="settings-actions">
            <button type="button" className="btn btn--ghost" onClick={handleReset}>
              Reset to defaults
            </button>
          </div>

          {saved ? (
            <p className="settings-saved" role="status">
              Preferences saved.
            </p>
          ) : null}
        </MarbleCard>
      </div>
    </>
  );
}

function connectionHints(err: unknown): string[] {
  const hints = [
    "Ensure the dashboard server is running and proxying to Platform.",
    "Check ACTORDOCK_PLATFORM_URL points at the Platform Service.",
    "Verify ACTORDOCK_API_KEY matches the Platform API key.",
  ];
  if (err instanceof PlatformAPIError && err.status === 401) {
    hints.unshift("API key rejected — confirm ACTORDOCK_API_KEY is correct.");
  }
  if (err instanceof PlatformAPIError && err.status === 503) {
    hints.unshift("Dashboard proxy reports API key is not configured.");
  }
  return hints;
}
