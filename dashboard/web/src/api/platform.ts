import {
  type HealthResponse,
  PlatformAPIError,
  type ConnectSandboxResponse,
  type CreateSandboxRequest,
  type CreateSandboxResponse,
  type Sandbox,
  type SandboxDetail,
  type SandboxLogEntry,
  type SandboxLogsV2Response,
  type SandboxMetric,
  type SandboxNetworkUpdate,
  type SandboxesMetricsResponse,
  type Snapshot,
  type Template,
  type TemplateDetail,
  type TemplateTag,
  type Volume,
  type VolumeDetail,
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
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return undefined as T;
  }
  const text = await res.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

function jsonBody(body: unknown): RequestInit {
  return {
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
}

export async function fetchHealth(): Promise<HealthResponse> {
  return request<HealthResponse>("/health");
}

export async function fetchSandboxes(): Promise<Sandbox[]> {
  return request<Sandbox[]>("/sandboxes");
}

export async function createSandbox(
  body: CreateSandboxRequest,
): Promise<CreateSandboxResponse> {
  return request<CreateSandboxResponse>("/sandboxes", {
    method: "POST",
    ...jsonBody(body),
  });
}

export async function killSandbox(sandboxID: string): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(sandboxID)}`, {
    method: "DELETE",
  });
}

export async function pauseSandbox(sandboxID: string): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(sandboxID)}/pause`, {
    method: "POST",
  });
}

export async function resumeSandbox(
  sandboxID: string,
  body: { timeout?: number } = {},
): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(sandboxID)}/resume`, {
    method: "POST",
    ...jsonBody(body),
  });
}

export async function refreshSandboxTTL(
  sandboxID: string,
  duration?: number,
): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(sandboxID)}/refreshes`, {
    method: "POST",
    ...jsonBody(duration !== undefined ? { duration } : {}),
  });
}

export async function setSandboxTimeout(
  sandboxID: string,
  timeout: number,
): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(sandboxID)}/timeout`, {
    method: "POST",
    ...jsonBody({ timeout }),
  });
}

export async function createSandboxSnapshot(
  sandboxID: string,
  name?: string,
): Promise<Snapshot> {
  return request<Snapshot>(
    `/sandboxes/${encodeURIComponent(sandboxID)}/snapshots`,
    {
      method: "POST",
      ...jsonBody(name ? { name } : {}),
    },
  );
}

export async function updateSandboxNetwork(
  sandboxID: string,
  body: SandboxNetworkUpdate,
): Promise<void> {
  await request<void>(`/sandboxes/${encodeURIComponent(sandboxID)}/network`, {
    method: "PUT",
    ...jsonBody(body),
  });
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

export async function fetchTemplate(templateID: string): Promise<TemplateDetail> {
  return request<TemplateDetail>(
    `/templates/${encodeURIComponent(templateID)}`,
  );
}

export async function fetchTemplateTags(
  templateID: string,
): Promise<TemplateTag[]> {
  return request<TemplateTag[]>(
    `/templates/${encodeURIComponent(templateID)}/tags`,
  );
}

export async function connectSandbox(
  sandboxID: string,
  timeout = 600,
): Promise<ConnectSandboxResponse> {
  return request<ConnectSandboxResponse>(
    `/sandboxes/${encodeURIComponent(sandboxID)}/connect`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ timeout }),
    },
  );
}

export async function fetchVolumes(): Promise<Volume[]> {
  return request<Volume[]>("/volumes");
}

export async function fetchVolume(volumeID: string): Promise<VolumeDetail> {
  return request<VolumeDetail>(`/volumes/${encodeURIComponent(volumeID)}`);
}

export async function fetchSnapshots(
  sandboxID?: string,
): Promise<Snapshot[]> {
  const params = new URLSearchParams({ limit: "100" });
  if (sandboxID) {
    params.set("sandboxID", sandboxID);
  }
  return request<Snapshot[]>(`/snapshots?${params.toString()}`);
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
