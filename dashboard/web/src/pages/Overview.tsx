import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  fetchHealth,
  fetchSandboxes,
  fetchTemplates,
  fetchVolumes,
} from "../api/platform";
import type { Sandbox } from "../api/types";
import {
  DataTable,
  MarbleCard,
  PageHeader,
  StatusBadge,
  type DataTableColumn,
} from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import {
  countByState,
  formatDateTime,
  recentSandboxes,
  sandboxStatusVariant,
} from "../utils/sandbox";
import "./Overview.css";

type OverviewData = {
  platformOk: boolean;
  sandboxes: Sandbox[];
  templateCount: number;
  volumeCount: number;
};

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; data: OverviewData }
  | { kind: "error"; message: string };

const quickLinks = [
  { label: "Sandboxes", path: "/sandboxes", desc: "List and inspect workloads" },
  { label: "Templates", path: "/templates", desc: "Browse template catalog" },
  { label: "Volumes", path: "/volumes", desc: "Persistent volume registry" },
  { label: "Snapshots", path: "/snapshots", desc: "Sandbox snapshot history" },
];

export function Overview() {
  const refreshMs = useRefreshIntervalMs();
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    let cancelled = false;

    async function loadOverview() {
      try {
        const [health, sandboxes, templates, volumes] = await Promise.all([
          fetchHealth().catch(() => null),
          fetchSandboxes().catch(() => [] as Sandbox[]),
          fetchTemplates().catch(() => []),
          fetchVolumes().catch(() => []),
        ]);

        if (cancelled) {
          return;
        }

        setState({
          kind: "ready",
          data: {
            platformOk: health?.status === "ok",
            sandboxes,
            templateCount: templates.length,
            volumeCount: volumes.length,
          },
        });
        setLastUpdated(new Date());
      } catch (err) {
        if (cancelled) {
          return;
        }
        setState({
          kind: "error",
          message:
            err instanceof Error ? err.message : "Failed to load overview",
        });
      }
    }

    void loadOverview();
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

  if (state.kind === "loading") {
    return (
      <PageHeader
        title="Overview"
        subtitle="Loading cluster summary…"
      />
    );
  }

  if (state.kind === "error") {
    return (
      <>
        <PageHeader title="Overview" subtitle="Cluster summary" />
        <div className="overview-error" role="alert">
          {state.message}
        </div>
      </>
    );
  }

  const { data } = state;
  const stateCounts = countByState(data.sandboxes);
  const recent = recentSandboxes(data.sandboxes);

  const columns: DataTableColumn<Sandbox>[] = [
    {
      key: "id",
      header: "Sandbox ID",
      mono: true,
      render: (row) => (
        <Link to={`/sandboxes/${row.sandboxID}`} className="overview-link">
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
      key: "startedAt",
      header: "Started",
      render: (row) => formatDateTime(row.startedAt),
    },
  ];

  return (
    <>
      <PageHeader
        title="Overview"
        subtitle="Sandbox counts, platform health, and recent activity."
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

      {lastUpdated ? (
        <p className="overview-updated">
          Last updated {lastUpdated.toLocaleTimeString()}
          {refreshMs > 0
            ? ` · auto-refresh every ${refreshMs / 1000}s`
            : " · auto-refresh off"}
        </p>
      ) : null}

      <section className="overview-metrics">
        <MarbleCard title="Sandboxes">
          <div className="overview-metric">
            <span className="overview-metric__value">{data.sandboxes.length}</span>
            <span className="overview-metric__label">total</span>
          </div>
          <div className="overview-metric-breakdown">
            {Object.entries(stateCounts).map(([state, count]) => (
              <span key={state}>
                {count} {state}
              </span>
            ))}
            {data.sandboxes.length === 0 ? (
              <span className="overview-muted">No sandboxes yet</span>
            ) : null}
          </div>
        </MarbleCard>

        <MarbleCard title="Platform">
          <div className="overview-metric">
            <span
              className={`overview-metric__value overview-metric__value--${data.platformOk ? "ok" : "bad"}`}
            >
              {data.platformOk ? "Healthy" : "Unreachable"}
            </span>
          </div>
        </MarbleCard>

        <MarbleCard title="Templates">
          <div className="overview-metric">
            <span className="overview-metric__value">{data.templateCount}</span>
            <span className="overview-metric__label">registered</span>
          </div>
        </MarbleCard>

        <MarbleCard title="Volumes">
          <div className="overview-metric">
            <span className="overview-metric__value">{data.volumeCount}</span>
            <span className="overview-metric__label">registered</span>
          </div>
        </MarbleCard>
      </section>

      <section className="overview-section">
        <h3 className="overview-heading">Recent sandboxes</h3>
        <DataTable
          columns={columns}
          rows={recent}
          rowKey={(row) => row.sandboxID}
          emptyMessage="No sandboxes yet. Create one from the Sandboxes page."
        />
      </section>

      <section className="overview-section">
        <h3 className="overview-heading">Quick links</h3>
        <div className="overview-links">
          {quickLinks.map((link) => (
            <Link key={link.path} to={link.path} className="overview-quick-link">
              <span className="overview-quick-link__title">{link.label}</span>
              <span className="overview-quick-link__desc">{link.desc}</span>
            </Link>
          ))}
        </div>
      </section>
    </>
  );
}
