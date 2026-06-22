import type { Snapshot } from "../api/types";

export function filterSnapshots(
  snapshots: Snapshot[],
  search: string,
): Snapshot[] {
  const query = search.trim().toLowerCase();
  if (!query) {
    return snapshots;
  }
  return snapshots.filter((snapshot) => {
    const haystack = [
      snapshot.snapshotID,
      snapshot.sandboxID ?? "",
      ...snapshot.names,
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(query);
  });
}

export function sortSnapshots(
  snapshots: Snapshot[],
  sortDir: "asc" | "desc",
): Snapshot[] {
  const sign = sortDir === "asc" ? 1 : -1;
  return [...snapshots].sort(
    (a, b) => sign * (dateValue(a.createdAt) - dateValue(b.createdAt)),
  );
}

export function snapshotDisplayName(snapshot: Snapshot): string {
  if (snapshot.names.length > 0) {
    return snapshot.names[0];
  }
  return snapshot.snapshotID;
}

function dateValue(iso?: string): number {
  if (!iso) {
    return 0;
  }
  const value = new Date(iso).getTime();
  return Number.isNaN(value) ? 0 : value;
}
