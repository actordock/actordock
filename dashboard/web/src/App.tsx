import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components";
import { PlaceholderPage } from "./pages/PlaceholderPage";
import { ThemePreview } from "./pages/ThemePreview";

export default function App() {
  return (
    <AppShell connectionStatus="checking">
      <Routes>
        <Route
          path="/"
          element={
            <PlaceholderPage
              title="Overview"
              subtitle="Sandbox counts and platform health — coming in WP3."
            />
          }
        />
        <Route
          path="/sandboxes"
          element={
            <PlaceholderPage
              title="Sandboxes"
              subtitle="Sandbox list and filters — coming in WP4."
            />
          }
        />
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
        <Route
          path="/settings"
          element={
            <PlaceholderPage
              title="Settings"
              subtitle="Platform connection — coming in WP2."
            />
          }
        />
        <Route path="/theme-preview" element={<ThemePreview />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppShell>
  );
}
