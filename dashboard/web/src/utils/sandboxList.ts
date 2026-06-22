import type { Sandbox, SandboxMetric, SandboxRow } from "../api/types";

export type SortKey = "startedAt" | "endAt" | "state";
export type SortDir = "asc" | "desc";

export type SandboxFilters = {
  states: string[];
  templateID: string;
  search: string;
};

export function mergeSandboxMetrics(
  sandboxes: Sandbox[],
  metrics: Record<string, SandboxMetric>,
): SandboxRow[] {
  return sandboxes.map((sb) => ({
    ...sb,
    metrics: metrics[sb.sandboxID],
  }));
}

export function uniqueTemplates(sandboxes: Sandbox[]): string[] {
  const ids = new Set<string>();
  for (const sb of sandboxes) {
    ids.add(sb.templateID);
  }
  return [...ids].sort();
}

export function uniqueStates(sandboxes: Sandbox[]): string[] {
  const states = new Set<string>();
  for (const sb of sandboxes) {
    states.add(sb.state.toLowerCase());
  }
  return [...states].sort();
}

export function filterSandboxes(
  rows: SandboxRow[],
  filters: SandboxFilters,
): SandboxRow[] {
  const search = filters.search.trim().toLowerCase();
  return rows.filter((row) => {
    if (filters.states.length > 0 && !filters.states.includes(row.state.toLowerCase())) {
      return false;
    }
    if (filters.templateID && row.templateID !== filters.templateID) {
      return false;
    }
    if (search && !row.sandboxID.toLowerCase().includes(search)) {
      return false;
    }
    return true;
  });
}

export function sortSandboxes(
  rows: SandboxRow[],
  sortKey: SortKey,
  sortDir: SortDir,
): SandboxRow[] {
  const sorted = [...rows];
  const sign = sortDir === "asc" ? 1 : -1;

  sorted.sort((a, b) => {
    switch (sortKey) {
      case "state":
        return sign * a.state.localeCompare(b.state);
      case "endAt":
        return sign * (dateValue(a.endAt) - dateValue(b.endAt));
      case "startedAt":
      default:
        return sign * (dateValue(a.startedAt) - dateValue(b.startedAt));
    }
  });

  return sorted;
}

function dateValue(iso: string): number {
  const value = new Date(iso).getTime();
  return Number.isNaN(value) ? 0 : value;
}

export function formatCpuPct(pct?: number): string {
  if (pct === undefined) {
    return "—";
  }
  return `${pct.toFixed(1)}%`;
}

export function formatMemUsage(used?: number, total?: number): string {
  if (used === undefined || total === undefined || total === 0) {
    return "—";
  }
  const toMB = (bytes: number) => bytes / (1024 * 1024);
  return `${toMB(used).toFixed(0)}/${toMB(total).toFixed(0)} MB`;
}
