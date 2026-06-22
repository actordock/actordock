import {
  type HealthResponse,
  PlatformAPIError,
  type Sandbox,
  type SandboxDetail,
  type SandboxLogEntry,
  type SandboxLogsV2Response,
  type SandboxMetric,
  type SandboxesMetricsResponse,
  type Template,
  type Volume,
} from "./types";

const API_PREFIX = "/api/platform";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_PREFIX}${path}`, init);
  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = (await res.json()) as { message?: string };
      if (body.message) {
        message = body.message;
      }
    } catch {
      // ignore non-JSON error bodies
    }
    throw new PlatformAPIError(message, res.status);
  }
  return res.json() as Promise<T>;
}

export async function fetchHealth(): Promise<HealthResponse> {
  return request<HealthResponse>("/health");
}

export async function fetchSandboxes(): Promise<Sandbox[]> {
  return request<Sandbox[]>("/sandboxes");
}

export async function fetchSandbox(sandboxID: string): Promise<SandboxDetail> {
  return request<SandboxDetail>(`/sandboxes/${encodeURIComponent(sandboxID)}`);
}

export async function fetchSandboxMetricSamples(
  sandboxID: string,
): Promise<SandboxMetric[]> {
  return request<SandboxMetric[]>(
    `/sandboxes/${encodeURIComponent(sandboxID)}/metrics`,
  );
}

export type SandboxLogsQuery = {
  limit?: number;
  level?: string;
  search?: string;
  cursor?: number;
  direction?: "forward" | "backward";
};

export async function fetchSandboxLogsV2(
  sandboxID: string,
  query: SandboxLogsQuery = {},
): Promise<SandboxLogEntry[]> {
  const params = new URLSearchParams();
  if (query.limit !== undefined) {
    params.set("limit", String(query.limit));
  }
  if (query.level) {
    params.set("level", query.level);
  }
  if (query.search) {
    params.set("search", query.search);
  }
  if (query.cursor !== undefined) {
    params.set("cursor", String(query.cursor));
  }
  if (query.direction) {
    params.set("direction", query.direction);
  }
  const qs = params.toString();
  const path = `/v2/sandboxes/${encodeURIComponent(sandboxID)}/logs${qs ? `?${qs}` : ""}`;
  const resp = await request<SandboxLogsV2Response>(path);
  return resp.logs ?? [];
}

const METRICS_BATCH_SIZE = 100;

export async function fetchSandboxesMetrics(
  sandboxIds: string[],
): Promise<Record<string, SandboxMetric>> {
  if (sandboxIds.length === 0) {
    return {};
  }

  const merged: Record<string, SandboxMetric> = {};
  for (let i = 0; i < sandboxIds.length; i += METRICS_BATCH_SIZE) {
    const chunk = sandboxIds.slice(i, i + METRICS_BATCH_SIZE);
    const qs = new URLSearchParams({ sandbox_ids: chunk.join(",") });
    const resp = await request<SandboxesMetricsResponse>(
      `/sandboxes/metrics?${qs.toString()}`,
    );
    Object.assign(merged, resp.sandboxes);
  }
  return merged;
}

export async function fetchTemplates(): Promise<Template[]> {
  return request<Template[]>("/templates");
}

export async function fetchVolumes(): Promise<Volume[]> {
  return request<Volume[]>("/volumes");
}

export async function testConnection(): Promise<{
  health: HealthResponse;
  sandboxes: Sandbox[];
}> {
  const health = await fetchHealth();
  const sandboxes = await fetchSandboxes();
  return { health, sandboxes };
}

export { PlatformAPIError };
