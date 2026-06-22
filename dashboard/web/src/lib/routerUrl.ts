const DEFAULT_ROUTER_URL = "/api/router";
const DEFAULT_ENVD_PORT = 80;

export function getRouterBaseUrl(): string {
  const configured = import.meta.env.VITE_ROUTER_URL?.trim();
  if (configured) {
    return configured.replace(/\/$/, "");
  }
  return DEFAULT_ROUTER_URL;
}

export function buildSandboxConnectUrl(
  sandboxID: string,
  domain: string,
  envdPort = DEFAULT_ENVD_PORT,
): string {
  if (domain && domain !== "localhost") {
    return `https://${envdPort}-${sandboxID}.${domain}`;
  }
  return `${getRouterBaseUrl()} (header E2b-Sandbox-Id: ${sandboxID})`;
}

export const SANDBOX_ID_HEADER = "E2b-Sandbox-Id";
