import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { fetchSandboxMetricSamples } from "../api/platform";
import type { SandboxDetail, SandboxMetric } from "../api/types";
import { MarbleCard } from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import { formatCpuPct, formatMemUsage } from "../utils/sandboxList";
import {
  formatBytes,
  latestMetric,
  pctOf,
  pushRingBuffer,
  sparklinePath,
} from "../utils/sandboxMetrics";
import { useSandboxDetail } from "./sandboxDetailContext";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; samples: SandboxMetric[] }
  | { kind: "error"; message: string };

export function SandboxDetailMetrics() {
  const { sandbox } = useSandboxDetail();
  if (!sandbox) {
    return null;
  }
  return <SandboxDetailMetricsBody key={sandbox.sandboxID} sandbox={sandbox} />;
}

function SandboxDetailMetricsBody({ sandbox }: { sandbox: SandboxDetail }) {
  const refreshMs = useRefreshIntervalMs();
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [cpuHistory, setCpuHistory] = useState<number[]>([]);
  const [memHistory, setMemHistory] = useState<number[]>([]);
  const [diskHistory, setDiskHistory] = useState<number[]>([]);

  useEffect(() => {
    const sandboxID = sandbox.sandboxID;
    let cancelled = false;

    async function load() {
      try {
        const samples = await fetchSandboxMetricSamples(sandboxID);
        if (cancelled) {
          return;
        }
        const latest = latestMetric(samples);
        if (latest) {
          setCpuHistory((prev) => pushRingBuffer(prev, latest.cpuUsedPct));
          setMemHistory((prev) =>
            pushRingBuffer(prev, pctOf(latest.memUsed, latest.memTotal)),
          );
          setDiskHistory((prev) =>
            pushRingBuffer(prev, pctOf(latest.diskUsed, latest.diskTotal)),
          );
        }
        setState({ kind: "ready", samples });
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to load metrics",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [sandbox.sandboxID, reloadToken]);

  useEffect(() => {
    if (refreshMs <= 0) {
      return;
    }
    const id = window.setInterval(() => {
      setReloadToken((token) => token + 1);
    }, refreshMs);
    return () => window.clearInterval(id);
  }, [refreshMs]);

  if (state.kind === "loading" && cpuHistory.length === 0) {
    return <p className="sandbox-detail-muted">Loading metrics…</p>;
  }

  if (state.kind === "error" && cpuHistory.length === 0) {
    return (
      <div className="sandbox-detail-error" role="alert">
        {state.message}
      </div>
    );
  }

  const latest = latestMetric(state.kind === "ready" ? state.samples : []);

  return (
    <>
      <div className="sandbox-detail-metrics-toolbar">
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => setReloadToken((token) => token + 1)}
        >
          Refresh
        </button>
        <span className="sandbox-detail-muted">
          {refreshMs > 0
            ? `Auto-refresh every ${refreshMs / 1000}s`
            : "Auto-refresh off (see Settings)"}
        </span>
        <Link to="/monitoring" className="sandbox-detail-link">
          Cluster monitoring
        </Link>
      </div>

      <div className="sandbox-detail-metrics-grid">
        <MetricCard
          title="CPU"
          value={formatCpuPct(latest?.cpuUsedPct)}
          pct={latest?.cpuUsedPct ?? 0}
          history={cpuHistory}
          subtitle={`${latest?.cpuCount ?? sandbox.cpuCount} vCPU`}
        />
        <MetricCard
          title="Memory"
          value={formatMemUsage(latest?.memUsed, latest?.memTotal)}
          pct={pctOf(latest?.memUsed, latest?.memTotal)}
          history={memHistory}
          subtitle={
            latest
              ? `${formatBytes(latest.memCache)} cache`
              : `${sandbox.memoryMB} MB allocated`
          }
        />
        <MetricCard
          title="Disk"
          value={`${formatBytes(latest?.diskUsed)} / ${formatBytes(latest?.diskTotal)}`}
          pct={pctOf(latest?.diskUsed, latest?.diskTotal)}
          history={diskHistory}
          subtitle={`${sandbox.diskSizeMB} MB allocated`}
        />
      </div>

      {latest ? (
        <p className="sandbox-detail-muted sandbox-detail-metrics-updated">
          Snapshot at {new Date(latest.timestamp).toLocaleString()}
        </p>
      ) : null}
    </>
  );
}

function MetricCard({
  title,
  value,
  pct,
  history,
  subtitle,
}: {
  title: string;
  value: string;
  pct: number;
  history: number[];
  subtitle: string;
}) {
  return (
    <MarbleCard title={title}>
      <div className="metric-gauge">
        <div className="metric-gauge__value">{value}</div>
        <div className="metric-gauge__bar" aria-hidden="true">
          <span style={{ width: `${Math.min(100, pct)}%` }} />
        </div>
        <svg
          className="metric-gauge__sparkline"
          viewBox="0 0 120 28"
          preserveAspectRatio="none"
          aria-hidden="true"
        >
          <path d={sparklinePath(history, 120, 28)} fill="none" stroke="currentColor" />
        </svg>
        <div className="metric-gauge__subtitle">{subtitle}</div>
      </div>
    </MarbleCard>
  );
}
