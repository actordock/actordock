import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components";
import { ToastProvider } from "./components/Toast/ToastProvider";
import { useConnectionStatus } from "./hooks/useConnectionStatus";
import { Monitoring } from "./pages/Monitoring";
import { Overview } from "./pages/Overview";
import { SandboxDetail } from "./pages/SandboxDetail";
import { SandboxDetailLogs } from "./pages/SandboxDetailLogs";
import { SandboxDetailMetrics } from "./pages/SandboxDetailMetrics";
import { SandboxDetailOverview } from "./pages/SandboxDetailOverview";
import { SandboxDetailTerminal } from "./pages/SandboxDetailTerminal";
import { Sandboxes } from "./pages/Sandboxes";
import { Settings } from "./pages/Settings";
import { TemplateDetail } from "./pages/TemplateDetail";
import { TemplateDetailBuilds } from "./pages/TemplateDetailBuilds";
import { TemplateDetailOverview } from "./pages/TemplateDetailOverview";
import { TemplateDetailTags } from "./pages/TemplateDetailTags";
import { Templates } from "./pages/Templates";
import { Snapshots } from "./pages/Snapshots";
import { VolumeDetail } from "./pages/VolumeDetail";
import { Volumes } from "./pages/Volumes";
import { ThemePreview } from "./pages/ThemePreview";

export default function App() {
  const connectionStatus = useConnectionStatus();

  return (
    <ToastProvider>
      <AppShell connectionStatus={connectionStatus}>
        <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/sandboxes" element={<Sandboxes />} />
        <Route path="/sandboxes/:id" element={<SandboxDetail />}>
          <Route index element={<SandboxDetailOverview />} />
          <Route path="metrics" element={<SandboxDetailMetrics />} />
          <Route path="logs" element={<SandboxDetailLogs />} />
          <Route path="terminal" element={<SandboxDetailTerminal />} />
        </Route>
        <Route path="/templates" element={<Templates />} />
        <Route path="/templates/:id" element={<TemplateDetail />}>
          <Route index element={<TemplateDetailOverview />} />
          <Route path="builds" element={<TemplateDetailBuilds />} />
          <Route path="tags" element={<TemplateDetailTags />} />
        </Route>
        <Route path="/volumes" element={<Volumes />} />
        <Route path="/volumes/:id" element={<VolumeDetail />} />
        <Route path="/snapshots" element={<Snapshots />} />
        <Route path="/sandboxes/monitoring" element={<Monitoring />} />
        <Route path="/monitoring" element={<Navigate to="/sandboxes/monitoring" replace />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/theme-preview" element={<ThemePreview />} />
        <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AppShell>
    </ToastProvider>
  );
}
