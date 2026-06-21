import type { ReactNode } from "react";
import { SideNav } from "../SideNav/SideNav";
import "./AppShell.css";

export type ConnectionStatus = "connected" | "disconnected" | "checking";

type AppShellProps = {
  children: ReactNode;
  connectionStatus?: ConnectionStatus;
};

export function AppShell({
  children,
  connectionStatus = "checking",
}: AppShellProps) {
  return (
    <div className="app-shell">
      <SideNav />
      <div className="app-shell__frame">
        <header className="app-shell__header">
          <div className="app-shell__header-inner">
            <h1 className="app-shell__title">Actordock</h1>
            <ConnectionPill status={connectionStatus} />
          </div>
        </header>
        <div className="app-shell__body">
          <div
            className="app-shell__bg"
            style={{ backgroundImage: "url(/column-bg.svg)" }}
            aria-hidden="true"
          />
          <main className="app-shell__main">
            <div className="app-shell__content page-enter">{children}</div>
          </main>
        </div>
      </div>
    </div>
  );
}

function ConnectionPill({ status }: { status: ConnectionStatus }) {
  const labels: Record<ConnectionStatus, string> = {
    connected: "Platform connected",
    disconnected: "Platform unreachable",
    checking: "Checking connection",
  };

  return (
    <span
      className={`connection-pill connection-pill--${status}`}
      role="status"
    >
      <span className="connection-pill__dot" aria-hidden="true" />
      {labels[status]}
    </span>
  );
}
