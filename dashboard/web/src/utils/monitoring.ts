import type { SandboxRow } from "../api/types";
import { pctOf } from "./sandboxMetrics";

export const HOT_METRIC_THRESHOLD = 80;

export type MonitoringSortKey = "cpu" | "memory" | "disk" | "sandboxID";

export function isHotSandbox(row: SandboxRow): boolean {
  const cpu = row.metrics?.cpuUsedPct ?? 0;
  const mem = pctOf(row.metrics?.memUsed, row.metrics?.memTotal);
  return cpu > HOT_METRIC_THRESHOLD || mem > HOT_METRIC_THRESHOLD;
}

export function sortMonitoringRows(
  rows: SandboxRow[],
  sortKey: MonitoringSortKey,
  sortDir: "asc" | "desc",
): SandboxRow[] {
  const sign = sortDir === "asc" ? 1 : -1;
  return [...rows].sort((a, b) => {
    switch (sortKey) {
      case "cpu":
        return (
          sign *
          ((a.metrics?.cpuUsedPct ?? 0) - (b.metrics?.cpuUsedPct ?? 0))
        );
      case "memory":
        return (
          sign *
          (pctOf(a.metrics?.memUsed, a.metrics?.memTotal) -
            pctOf(b.metrics?.memUsed, b.metrics?.memTotal))
        );
      case "disk":
        return (
          sign *
          (pctOf(a.metrics?.diskUsed, a.metrics?.diskTotal) -
            pctOf(b.metrics?.diskUsed, b.metrics?.diskTotal))
        );
      case "sandboxID":
      default:
        return sign * a.sandboxID.localeCompare(b.sandboxID);
    }
  });
}

export function hotSandboxCount(rows: SandboxRow[]): number {
  return rows.filter(isHotSandbox).length;
}
