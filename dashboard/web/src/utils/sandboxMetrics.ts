import type { SandboxMetric } from "../api/types";

const RING_BUFFER_MAX = 60;

export function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes < 0) {
    return "—";
  }
  if (bytes > 1e15) {
    return "unlimited";
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  if (bytes < 1024 * 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function pctOf(used?: number, total?: number): number {
  if (used === undefined || total === undefined || total <= 0) {
    return 0;
  }
  return Math.min(100, (used / total) * 100);
}

export function latestMetric(samples: SandboxMetric[]): SandboxMetric | undefined {
  if (samples.length === 0) {
    return undefined;
  }
  return samples[samples.length - 1];
}

export function pushRingBuffer(
  buffer: number[],
  value: number,
  maxSize = RING_BUFFER_MAX,
): number[] {
  const next = [...buffer, value];
  if (next.length > maxSize) {
    return next.slice(next.length - maxSize);
  }
  return next;
}

export function sparklinePath(
  values: number[],
  width: number,
  height: number,
): string {
  if (values.length === 0) {
    return "";
  }
  const max = Math.max(...values, 1);
  const step = values.length <= 1 ? 0 : width / (values.length - 1);
  const points = values.map((value, index) => {
    const x = index * step;
    const y = height - (value / max) * (height - 4) - 2;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  return `M ${points.join(" L ")}`;
}
