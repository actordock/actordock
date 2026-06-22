import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { Link, NavLink, Outlet, useParams } from "react-router-dom";
import { fetchTemplate } from "../api/platform";
import type { TemplateDetail } from "../api/types";
import { StatusBadge } from "../components";
import { buildStatusVariant } from "../utils/template";
import { TemplateDetailContext } from "./templateDetailContext";
import "./TemplateDetail.css";

type DetailState =
  | { kind: "loading" }
  | { kind: "ready"; template: TemplateDetail }
  | { kind: "error"; message: string };

const tabs = [
  { label: "Overview", path: "" },
  { label: "Builds", path: "builds" },
  { label: "Tags", path: "tags" },
];

export function TemplateDetail() {
  const { id = "" } = useParams();
  const [state, setState] = useState<DetailState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);

  const reload = useCallback(() => {
    setReloadToken((token) => token + 1);
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setState({ kind: "loading" });
      try {
        const template = await fetchTemplate(id);
        if (!cancelled) {
          setState({ kind: "ready", template });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to load template",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [id, reloadToken]);

  const contextValue = useMemo(
    () => ({
      template: state.kind === "ready" ? state.template : null,
      reload,
    }),
    [state, reload],
  );

  if (state.kind === "loading") {
    return <p className="template-detail-loading">Loading template…</p>;
  }

  if (state.kind === "error") {
    return (
      <div className="template-detail-error" role="alert">
        <p>{state.message}</p>
        <Link to="/templates" className="template-detail-link">
          Back to templates
        </Link>
      </div>
    );
  }

  const { template } = state;
  const buildStatus = template.builds[0]?.status ?? "unknown";

  return (
    <TemplateDetailContext.Provider value={contextValue}>
      <nav className="template-detail-breadcrumb" aria-label="Breadcrumb">
        <Link to="/templates">Templates</Link>
        <span aria-hidden="true">/</span>
        <span>{template.templateID}</span>
      </nav>

      <header className="template-detail-header">
        <div>
          <h2 className="template-detail-title">{template.templateID}</h2>
          <div className="template-detail-header__meta">
            <StatusBadge
              variant={buildStatusVariant(buildStatus)}
              label={buildStatus}
            />
            <span className="template-detail-muted">
              {template.public ? "Public" : "Private"}
            </span>
          </div>
        </div>
        <button
          type="button"
          className="btn btn--ghost"
          disabled
          title="Template builds are managed via Substrate (backlog)"
        >
          Build template
        </button>
      </header>

      <nav className="template-detail-tabs" aria-label="Template sections">
        {tabs.map((tab) => (
          <NavLink
            key={tab.label}
            to={tab.path === "" ? `/templates/${id}` : `/templates/${id}/${tab.path}`}
            end={tab.path === ""}
            className={({ isActive }) =>
              `template-detail-tab${isActive ? " template-detail-tab--active" : ""}`
            }
          >
            {tab.label}
          </NavLink>
        ))}
      </nav>

      <div className="template-detail-content">
        <Outlet />
      </div>
    </TemplateDetailContext.Provider>
  );
}

export function DetailField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="template-detail-field">
      <span className="template-detail-field__label">{label}</span>
      <span className="template-detail-field__value">{children}</span>
    </div>
  );
}
