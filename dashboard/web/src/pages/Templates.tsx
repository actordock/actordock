import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { fetchTemplates } from "../api/platform";
import type { Template } from "../api/types";
import { DataTable, PageHeader, StatusBadge, type DataTableColumn } from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import {
  buildStatusVariant,
  filterTemplates,
  formatTemplateResources,
  sortTemplates,
} from "../utils/template";
import "./Templates.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; rows: Template[] }
  | { kind: "error"; message: string };

export function Templates() {
  const navigate = useNavigate();
  const refreshMs = useRefreshIntervalMs();
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"templateID" | "buildStatus" | "createdAt">(
    "templateID",
  );
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const templates = await fetchTemplates();
        if (!cancelled) {
          setLoadState({ kind: "ready", rows: templates });
        }
      } catch (err) {
        if (!cancelled) {
          setLoadState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to load templates",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  useEffect(() => {
    if (refreshMs <= 0) {
      return;
    }
    const id = window.setInterval(() => {
      setReloadToken((token) => token + 1);
    }, refreshMs);
    return () => window.clearInterval(id);
  }, [refreshMs]);

  const visibleRows = useMemo(() => {
    if (loadState.kind !== "ready") {
      return [];
    }
    return sortTemplates(
      filterTemplates(loadState.rows, search),
      sortKey,
      sortDir,
    );
  }, [loadState, search, sortKey, sortDir]);

  const columns: DataTableColumn<Template>[] = [
    {
      key: "templateID",
      header: "Template",
      width: "22%",
      render: (row) => row.templateID,
    },
    {
      key: "buildStatus",
      header: "Build status",
      render: (row) => (
        <StatusBadge
          variant={buildStatusVariant(row.buildStatus)}
          label={row.buildStatus}
        />
      ),
    },
    {
      key: "resources",
      header: "Resources",
      render: (row) => formatTemplateResources(row),
    },
    {
      key: "envdVersion",
      header: "Envd",
      render: (row) => row.envdVersion || "—",
    },
    {
      key: "public",
      header: "Public",
      render: (row) => (row.public ? "Yes" : "No"),
    },
    {
      key: "aliases",
      header: "Aliases",
      render: (row) => row.aliases?.join(", ") || "—",
    },
  ];

  return (
    <>
      <PageHeader
        title="Templates"
        subtitle="Read-only catalog of cluster templates and builds."
        actions={
          <button
            type="button"
            className="btn btn--ghost"
            onClick={() => setReloadToken((token) => token + 1)}
          >
            Refresh
          </button>
        }
      />

      <p className="templates-note">
        Template build and upload are not available in the dashboard yet.
      </p>

      {loadState.kind === "error" ? (
        <div className="templates-error" role="alert">
          {loadState.message}
        </div>
      ) : null}

      <section className="templates-toolbar">
        <label className="templates-field">
          <span>Search</span>
          <input
            type="search"
            placeholder="template id, alias, status"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>

        <label className="templates-field">
          <span>Sort by</span>
          <select
            value={`${sortKey}:${sortDir}`}
            onChange={(e) => {
              const [key, dir] = e.target.value.split(":") as [
                typeof sortKey,
                typeof sortDir,
              ];
              setSortKey(key);
              setSortDir(dir);
            }}
          >
            <option value="templateID:asc">Template (A–Z)</option>
            <option value="templateID:desc">Template (Z–A)</option>
            <option value="buildStatus:asc">Status (A–Z)</option>
            <option value="buildStatus:desc">Status (Z–A)</option>
            <option value="createdAt:desc">Created (newest)</option>
            <option value="createdAt:asc">Created (oldest)</option>
          </select>
        </label>
      </section>

      {loadState.kind === "loading" ? (
        <p className="templates-loading">Loading templates…</p>
      ) : visibleRows.length === 0 ? (
        <div className="templates-empty">
          <h3>
            {loadState.kind === "ready" && loadState.rows.length > 0
              ? "No templates match search"
              : "No templates yet"}
          </h3>
          <p>Templates are provisioned via ActorTemplate resources in the cluster.</p>
        </div>
      ) : (
        <DataTable
          columns={columns}
          rows={visibleRows}
          rowKey={(row) => row.templateID}
          onRowClick={(row) => navigate(`/templates/${row.templateID}`)}
        />
      )}
    </>
  );
}
