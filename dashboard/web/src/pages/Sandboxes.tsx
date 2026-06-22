import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { fetchSandboxesMetrics, fetchSandboxes } from "../api/platform";
import type { SandboxRow } from "../api/types";
import { DataTable, PageHeader, StatusBadge, type DataTableColumn } from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import { formatDateTime, sandboxStatusVariant } from "../utils/sandbox";
import {
  filterSandboxes,
  formatCpuPct,
  formatMemUsage,
  mergeSandboxMetrics,
  sortSandboxes,
  type SandboxFilters,
  type SortDir,
  type SortKey,
  uniqueStates,
  uniqueTemplates,
} from "../utils/sandboxList";
import "./Sandboxes.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; rows: SandboxRow[] }
  | { kind: "error"; message: string };

const defaultFilters: SandboxFilters = {
  states: [],
  templateID: "",
  search: "",
};

export function Sandboxes() {
  const navigate = useNavigate();
  const refreshMs = useRefreshIntervalMs();
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [filters, setFilters] = useState<SandboxFilters>(defaultFilters);
  const [sortKey, setSortKey] = useState<SortKey>("startedAt");
  const [sortDir, setSortDir] = useState<SortDir>("desc");
  const [copiedId, setCopiedId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const sandboxes = await fetchSandboxes();
        const metrics = await fetchSandboxesMetrics(
          sandboxes.map((sb) => sb.sandboxID),
        );
        if (cancelled) {
          return;
        }
        setLoadState({
          kind: "ready",
          rows: mergeSandboxMetrics(sandboxes, metrics),
        });
      } catch (err) {
        if (cancelled) {
          return;
        }
        setLoadState({
          kind: "error",
          message: err instanceof Error ? err.message : "Failed to load sandboxes",
        });
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

  const allRows = useMemo(
    () => (loadState.kind === "ready" ? loadState.rows : []),
    [loadState],
  );
  const templates = useMemo(() => uniqueTemplates(allRows), [allRows]);
  const states = useMemo(() => uniqueStates(allRows), [allRows]);
  const visibleRows = useMemo(
    () => sortSandboxes(filterSandboxes(allRows, filters), sortKey, sortDir),
    [allRows, filters, sortKey, sortDir],
  );

  const columns: DataTableColumn<SandboxRow>[] = [
    {
      key: "id",
      header: "Sandbox ID",
      width: "28%",
      mono: true,
      render: (row) => (
        <div className="sandboxes-id-cell">
          <span className="sandboxes-id">{row.sandboxID}</span>
          <button
            type="button"
            className="sandboxes-copy"
            onClick={(event) => {
              event.stopPropagation();
              void copySandboxId(row.sandboxID, setCopiedId);
            }}
          >
            {copiedId === row.sandboxID ? "Copied" : "Copy"}
          </button>
        </div>
      ),
    },
    {
      key: "state",
      header: "State",
      render: (row) => <StatusBadge variant={sandboxStatusVariant(row.state)} />,
    },
    {
      key: "template",
      header: "Template",
      render: (row) => row.templateID,
    },
    {
      key: "alias",
      header: "Alias",
      render: (row) => row.alias || "—",
    },
    {
      key: "cpu",
      header: "CPU",
      render: (row) => formatCpuPct(row.metrics?.cpuUsedPct),
    },
    {
      key: "mem",
      header: "Memory",
      render: (row) => formatMemUsage(row.metrics?.memUsed, row.metrics?.memTotal),
    },
    {
      key: "startedAt",
      header: "Started",
      render: (row) => formatDateTime(row.startedAt),
    },
    {
      key: "endAt",
      header: "Ends",
      render: (row) => formatDateTime(row.endAt),
    },
  ];

  return (
    <>
      <PageHeader
        title="Sandboxes"
        subtitle="All sandboxes with live CPU and memory snapshots."
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

      {loadState.kind === "error" ? (
        <div className="sandboxes-error" role="alert">
          {loadState.message}
        </div>
      ) : null}

      <section className="sandboxes-toolbar">
        <label className="sandboxes-field">
          <span>Search ID</span>
          <input
            type="search"
            placeholder="sandbox id"
            value={filters.search}
            onChange={(e) =>
              setFilters((prev) => ({ ...prev, search: e.target.value }))
            }
          />
        </label>

        <label className="sandboxes-field">
          <span>Template</span>
          <select
            value={filters.templateID}
            onChange={(e) =>
              setFilters((prev) => ({ ...prev, templateID: e.target.value }))
            }
          >
            <option value="">All templates</option>
            {templates.map((templateID) => (
              <option key={templateID} value={templateID}>
                {templateID}
              </option>
            ))}
          </select>
        </label>

        <label className="sandboxes-field">
          <span>Sort by</span>
          <select
            value={`${sortKey}:${sortDir}`}
            onChange={(e) => {
              const [key, dir] = e.target.value.split(":") as [SortKey, SortDir];
              setSortKey(key);
              setSortDir(dir);
            }}
          >
            <option value="startedAt:desc">Started (newest)</option>
            <option value="startedAt:asc">Started (oldest)</option>
            <option value="endAt:desc">Ends (latest)</option>
            <option value="endAt:asc">Ends (earliest)</option>
            <option value="state:asc">State (A–Z)</option>
            <option value="state:desc">State (Z–A)</option>
          </select>
        </label>
      </section>

      {states.length > 0 ? (
        <section className="sandboxes-states">
          <span className="sandboxes-states__label">State</span>
          {states.map((state) => {
            const active = filters.states.includes(state);
            return (
              <button
                key={state}
                type="button"
                className={`sandboxes-state-chip${active ? " sandboxes-state-chip--active" : ""}`}
                onClick={() =>
                  setFilters((prev) => ({
                    ...prev,
                    states: active
                      ? prev.states.filter((s) => s !== state)
                      : [...prev.states, state],
                  }))
                }
              >
                {state}
              </button>
            );
          })}
          {filters.states.length > 0 ? (
            <button
              type="button"
              className="sandboxes-state-clear"
              onClick={() => setFilters((prev) => ({ ...prev, states: [] }))}
            >
              Clear
            </button>
          ) : null}
        </section>
      ) : null}

      {loadState.kind === "loading" ? (
        <p className="sandboxes-loading">Loading sandboxes…</p>
      ) : visibleRows.length === 0 ? (
        <EmptyState hasSandboxes={allRows.length > 0} />
      ) : (
        <DataTable
          columns={columns}
          rows={visibleRows}
          rowKey={(row) => row.sandboxID}
          onRowClick={(row) => navigate(`/sandboxes/${row.sandboxID}`)}
        />
      )}
    </>
  );
}

function EmptyState({ hasSandboxes }: { hasSandboxes: boolean }) {
  return (
    <div className="sandboxes-empty">
      <svg
        className="sandboxes-empty__art"
        viewBox="0 0 120 140"
        aria-hidden="true"
      >
        <path d="M30 120V50M90 120V50" stroke="currentColor" strokeWidth="2" />
        <path d="M20 50h80M18 120h84" stroke="currentColor" strokeWidth="2" />
        <ellipse cx="60" cy="42" rx="24" ry="8" stroke="currentColor" strokeWidth="1.5" fill="none" />
        <path d="M50 70c10-18 30-18 40 0" stroke="currentColor" strokeWidth="1.2" fill="none" opacity="0.5" />
      </svg>
      <h3>{hasSandboxes ? "No sandboxes match filters" : "No sandboxes yet"}</h3>
      <p>
        {hasSandboxes
          ? "Try clearing filters or search terms."
          : "Create a sandbox to start running workloads in the cluster."}
      </p>
      <button type="button" className="btn btn--primary" disabled title="Coming in WP11">
        Create sandbox
      </button>
    </div>
  );
}

async function copySandboxId(
  sandboxID: string,
  setCopiedId: (id: string | null) => void,
) {
  try {
    await navigator.clipboard.writeText(sandboxID);
    setCopiedId(sandboxID);
    window.setTimeout(() => setCopiedId(null), 1500);
  } catch {
    setCopiedId(null);
  }
}
