import type { Sandbox, Volume } from "../api/types";

export function filterVolumes(volumes: Volume[], search: string): Volume[] {
  const query = search.trim().toLowerCase();
  if (!query) {
    return volumes;
  }
  return volumes.filter((volume) => {
    const haystack = [volume.volumeID, volume.name].join(" ").toLowerCase();
    return haystack.includes(query);
  });
}

export function sortVolumes(
  volumes: Volume[],
  sortKey: "name" | "createdAt",
  sortDir: "asc" | "desc",
): Volume[] {
  const sign = sortDir === "asc" ? 1 : -1;
  return [...volumes].sort((a, b) => {
    switch (sortKey) {
      case "createdAt":
        return sign * (dateValue(a.createdAt) - dateValue(b.createdAt));
      case "name":
      default:
        return sign * a.name.localeCompare(b.name);
    }
  });
}

export type VolumeMountRef = {
  sandboxID: string;
  templateID: string;
  state: string;
  mountPath: string;
};

export function sandboxesUsingVolume(
  sandboxes: Sandbox[],
  volume: Pick<Volume, "volumeID" | "name">,
): VolumeMountRef[] {
  const refs: VolumeMountRef[] = [];
  for (const sandbox of sandboxes) {
    for (const mount of sandbox.volumeMounts ?? []) {
      if (mount.name === volume.name || mount.name === volume.volumeID) {
        refs.push({
          sandboxID: sandbox.sandboxID,
          templateID: sandbox.templateID,
          state: sandbox.state,
          mountPath: mount.path,
        });
      }
    }
  }
  return refs;
}

function dateValue(iso?: string): number {
  if (!iso) {
    return 0;
  }
  const value = new Date(iso).getTime();
  return Number.isNaN(value) ? 0 : value;
}
