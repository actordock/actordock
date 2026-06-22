import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Link, NavLink, Outlet, useParams } from "react-router-dom";
import { fetchSandbox } from "../api/platform";
import type { SandboxDetail } from "../api/types";
import { StatusBadge } from "../components";
import { sandboxStatusVariant } from "../utils/sandbox";
import { SandboxDetailContext } from "./sandboxDetailContext";
import { SandboxDetailActions } from "./SandboxDetailActions";
import "./SandboxDetail.css";

type DetailState =
  | { kind: "loading" }
  | { kind: "ready"; sandbox: SandboxDetail }
  | { kind: "error"; message: string };

const tabs = [
  { label: "Overview", path: "" },
  { label: "Metrics", path: "metrics" },
  { label: "Logs", path: "logs" },
  { label: "Terminal", path: "terminal" },
];

export function SandboxDetail() {
  const { id = "" } = useParams();
  const [state, setState] = useState<DetailState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const hasLoadedRef = useRef(false);

  const reload = useCallback(() => {
    setReloadToken((token) => token + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      if (!hasLoadedRef.current) {
        setState({ kind: "loading" });
      }
      try {
        const sandbox = await fetchSandbox(id);
        if (!cancelled) {
          hasLoadedRef.current = true;
          setState({ kind: "ready", sandbox });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to load sandbox",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [id, reloadToken]);

  useEffect(() => {
    hasLoadedRef.current = false;
  }, [id]);

  const contextValue = useMemo(
    () => ({
      sandbox: state.kind === "ready" ? state.sandbox : null,
      reload,
    }),
    [state, reload],
  );

  if (state.kind === "loading") {
    return <p className="sandbox-detail-loading">Loading sandbox…</p>;
  }

  if (state.kind === "error") {
    return (
      <div className="sandbox-detail-error" role="alert">
        <p>{state.message}</p>
        <Link to="/sandboxes" className="sandbox-detail-back">
          Back to sandboxes
        </Link>
      </div>
    );
  }

  const { sandbox } = state;

  return (
    <SandboxDetailContext.Provider value={contextValue}>
      <nav className="sandbox-detail-breadcrumb" aria-label="Breadcrumb">
        <Link to="/sandboxes">Sandboxes</Link>
        <span aria-hidden="true">/</span>
        <span className="mono">{truncateId(sandbox.sandboxID)}</span>
      </nav>

      <header className="sandbox-detail-header">
        <div className="sandbox-detail-header__main">
          <h2 className="sandbox-detail-title">
            <span className="mono">{sandbox.sandboxID}</span>
          </h2>
          <div className="sandbox-detail-header__meta">
            <StatusBadge variant={sandboxStatusVariant(sandbox.state)} />
            <span className="sandbox-detail-muted">{sandbox.templateID}</span>
          </div>
        </div>
        <SandboxDetailActions sandbox={sandbox} reload={reload} />
      </header>

      <nav className="sandbox-detail-tabs" aria-label="Sandbox sections">
        {tabs.map((tab) => (
          <NavLink
            key={tab.label}
            to={tab.path === "" ? `/sandboxes/${id}` : `/sandboxes/${id}/${tab.path}`}
            end={tab.path === ""}
            className={({ isActive }) =>
              `sandbox-detail-tab${isActive ? " sandbox-detail-tab--active" : ""}`
            }
          >
            {tab.label}
          </NavLink>
        ))}
      </nav>

      <div className="sandbox-detail-content">
        <Outlet />
      </div>
    </SandboxDetailContext.Provider>
  );
}

function truncateId(id: string): string {
  if (id.length <= 16) {
    return id;
  }
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

export function DetailField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="sandbox-detail-field">
      <span className="sandbox-detail-field__label">{label}</span>
      <span className="sandbox-detail-field__value">{children}</span>
    </div>
  );
}
