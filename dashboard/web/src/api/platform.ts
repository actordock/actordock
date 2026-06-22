import {
  type HealthResponse,
  PlatformAPIError,
  type Sandbox,
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
