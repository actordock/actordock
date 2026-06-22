import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { fetchSandboxes, fetchSandboxesMetrics } from "../api/platform";
import type { SandboxRow } from "../api/types";
import { DataTable, PageHeader, StatusBadge, type DataTableColumn } from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import { sandboxStatusVariant } from "../utils/sandbox";
import { formatCpuPct, formatMemUsage, mergeSandboxMetrics } from "../utils/sandboxList";
import { pctOf } from "../utils/sandboxMetrics";
import {
  HOT_METRIC_THRESHOLD,
  hotSandboxCount,
  isHotSandbox,
  sortMonitoringRows,
  type MonitoringSortKey,
} from "../utils/monitoring";
import "./Monitoring.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; rows: SandboxRow[] }
  | { kind: "error"; message: string };

export function Monitoring() {
  const navigate = useNavigate();
  const refreshMs = useRefreshIntervalMs();
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [sortKey, setSortKey] = useState<MonitoringSortKey>("cpu");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [hotOnly, setHotOnly] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const sandboxes = await fetchSandboxes();
        const metrics = await fetchSandboxesMetrics(
          sandboxes.map((sb) => sb.sandboxID),
        );
        if (!cancelled) {
          setLoadState({
            kind: "ready",
            rows: mergeSandboxMetrics(sandboxes, metrics),
          });
        }
      } catch (err) {
        if (!cancelled) {
          setLoadState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to load monitoring data",
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

  const allRows = useMemo(
    () => (loadState.kind === "ready" ? loadState.rows : []),
    [loadState],
  );
  const visibleRows = useMemo(() => {
    const sorted = sortMonitoringRows(allRows, sortKey, sortDir);
    if (!hotOnly) {
      return sorted;
    }
    return sorted.filter(isHotSandbox);
  }, [allRows, sortKey, sortDir, hotOnly]);

  const hotCount = useMemo(() => hotSandboxCount(allRows), [allRows]);

  const columns: DataTableColumn<SandboxRow>[] = [
    {
      key: "id",
      header: "Sandbox ID",
      width: "26%",
      mono: true,
      render: (row) => (
        <Link
          to={`/sandboxes/${row.sandboxID}/metrics`}
          className="monitoring-link"
          onClick={(e) => e.stopPropagation()}
        >
          {row.sandboxID}
        </Link>
      ),
    },
    {
      key: "template",
      header: "Template",
      render: (row) => row.templateID,
    },
    {
      key: "state",
      header: "State",
      render: (row) => (
        <StatusBadge variant={sandboxStatusVariant(row.state)} />
      ),
    },
    {
      key: "cpu",
      header: "CPU",
      render: (row) => (
        <MetricCell
          value={formatCpuPct(row.metrics?.cpuUsedPct)}
          hot={(row.metrics?.cpuUsedPct ?? 0) > HOT_METRIC_THRESHOLD}
        />
      ),
    },
    {
      key: "mem",
      header: "Memory",
      render: (row) => {
        const pct = pctOf(row.metrics?.memUsed, row.metrics?.memTotal);
        return (
          <MetricCell
            value={formatMemUsage(row.metrics?.memUsed, row.metrics?.memTotal)}
            hot={pct > HOT_METRIC_THRESHOLD}
          />
        );
      },
    },
    {
      key: "disk",
      header: "Disk",
      render: (row) => {
        const pct = pctOf(row.metrics?.diskUsed, row.metrics?.diskTotal);
        return (
          <MetricCell value={`${pct.toFixed(1)}%`} hot={pct > HOT_METRIC_THRESHOLD} />
        );
      },
    },
  ];

  return (
    <>
      <PageHeader
        title="Monitoring"
        subtitle="Aggregate CPU, memory, and disk usage across sandboxes."
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

      {loadState.kind === "ready" ? (
        <p className="monitoring-summary">
          {allRows.length} sandbox{allRows.length === 1 ? "" : "es"}
          {hotCount > 0 ? (
            <>
              {" "}
              · <span className="monitoring-summary__hot">{hotCount} hot</span> (
              {">"}
              {HOT_METRIC_THRESHOLD}% CPU or memory)
            </>
          ) : null}
          {refreshMs > 0 ? ` · auto-refresh every ${refreshMs / 1000}s` : ""}
        </p>
      ) : null}

      {loadState.kind === "error" ? (
        <div className="monitoring-error" role="alert">
          {loadState.message}
        </div>
      ) : null}

      <section className="monitoring-toolbar">
        <label className="monitoring-field">
          <span>Sort by</span>
          <select
            value={`${sortKey}:${sortDir}`}
            onChange={(e) => {
              const [key, dir] = e.target.value.split(":") as [
                MonitoringSortKey,
                "asc" | "desc",
              ];
              setSortKey(key);
              setSortDir(dir);
            }}
          >
            <option value="cpu:desc">CPU (highest)</option>
            <option value="cpu:asc">CPU (lowest)</option>
            <option value="memory:desc">Memory (highest)</option>
            <option value="memory:asc">Memory (lowest)</option>
            <option value="disk:desc">Disk (highest)</option>
            <option value="disk:asc">Disk (lowest)</option>
            <option value="sandboxID:asc">Sandbox ID (A–Z)</option>
          </select>
        </label>

        <label className="monitoring-checkbox">
          <input
            type="checkbox"
            checked={hotOnly}
            onChange={(e) => setHotOnly(e.target.checked)}
          />
          Hot only
        </label>
      </section>

      {loadState.kind === "loading" ? (
        <p className="monitoring-muted">Loading metrics…</p>
      ) : visibleRows.length === 0 ? (
        <div className="monitoring-empty">
          <h3>
            {hotOnly && allRows.length > 0
              ? "No hot sandboxes right now"
              : "No sandboxes to monitor"}
          </h3>
          <p>
            {hotOnly
              ? `Nothing is above ${HOT_METRIC_THRESHOLD}% CPU or memory.`
              : "Create a sandbox to see live resource usage."}
          </p>
        </div>
      ) : (
        <DataTable
          columns={columns}
          rows={visibleRows}
          rowKey={(row) => row.sandboxID}
          rowClassName={(row) =>
            isHotSandbox(row) ? "data-table__row--hot" : undefined
          }
          onRowClick={(row) => navigate(`/sandboxes/${row.sandboxID}/metrics`)}
          emptyMessage="No sandboxes to monitor."
        />
      )}
    </>
  );
}

function MetricCell({ value, hot }: { value: string; hot: boolean }) {
  return (
    <span className={hot ? "monitoring-metric monitoring-metric--hot" : "monitoring-metric"}>
      {value}
    </span>
  );
}
