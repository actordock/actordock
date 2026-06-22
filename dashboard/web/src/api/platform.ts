import {
  type HealthResponse,
  PlatformAPIError,
  type Sandbox,
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

const METRICS_BATCH_SIZE = 100;

export async function fetchSandboxMetrics(
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
