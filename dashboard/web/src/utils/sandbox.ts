import type { StatusVariant } from "../components/StatusBadge/StatusBadge";
import type { Sandbox } from "../api/types";

export function sandboxStatusVariant(state: string): StatusVariant {
  switch (state.toLowerCase()) {
    case "running":
      return "running";
    case "paused":
      return "paused";
    case "failed":
      return "failed";
    case "pending":
      return "starting";
    case "killed":
      return "killed";
    default:
      return "unknown";
  }
}

export function formatDateTime(iso: string): string {
  if (!iso) {
    return "—";
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleString();
}

export function recentSandboxes(sandboxes: Sandbox[], limit = 5): Sandbox[] {
  return [...sandboxes]
    .sort(
      (a, b) =>
        new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime(),
    )
    .slice(0, limit);
}

export function countByState(sandboxes: Sandbox[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const sb of sandboxes) {
    const key = sb.state.toLowerCase();
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}
