import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components";
import { useConnectionStatus } from "./hooks/useConnectionStatus";
import { Overview } from "./pages/Overview";
import { PlaceholderPage } from "./pages/PlaceholderPage";
import { SandboxDetail } from "./pages/SandboxDetail";
import { SandboxDetailLogs } from "./pages/SandboxDetailLogs";
import { SandboxDetailMetrics } from "./pages/SandboxDetailMetrics";
import { SandboxDetailOverview } from "./pages/SandboxDetailOverview";
import { Sandboxes } from "./pages/Sandboxes";
import { Settings } from "./pages/Settings";
import { ThemePreview } from "./pages/ThemePreview";

export default function App() {
  const connectionStatus = useConnectionStatus();

  return (
    <AppShell connectionStatus={connectionStatus}>
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/sandboxes" element={<Sandboxes />} />
        <Route path="/sandboxes/:id" element={<SandboxDetail />}>
          <Route index element={<SandboxDetailOverview />} />
          <Route path="metrics" element={<SandboxDetailMetrics />} />
          <Route path="logs" element={<SandboxDetailLogs />} />
        </Route>
        <Route
          path="/templates"
          element={
            <PlaceholderPage
              title="Templates"
              subtitle="Template catalog — coming in WP8."
            />
          }
        />
        <Route
          path="/volumes"
          element={
            <PlaceholderPage
              title="Volumes"
              subtitle="Volume list — coming in WP9."
            />
          }
        />
        <Route
          path="/snapshots"
          element={
            <PlaceholderPage
              title="Snapshots"
              subtitle="Snapshot list — coming in WP10."
            />
          }
        />
        <Route
          path="/monitoring"
          element={
            <PlaceholderPage
              title="Monitoring"
              subtitle="Aggregate metrics — coming in WP13."
            />
          }
        />
        <Route path="/settings" element={<Settings />} />
        <Route path="/theme-preview" element={<ThemePreview />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppShell>
  );
}
