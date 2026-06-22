export type HealthResponse = {
  status: string;
};

export type Sandbox = {
  sandboxID: string;
  templateID: string;
  state: string;
  startedAt: string;
  endAt: string;
  alias?: string;
  cpuCount: number;
  memoryMB: number;
};

export type SandboxMetric = {
  timestamp: string;
  timestampUnix: number;
  cpuCount: number;
  cpuUsedPct: number;
  memUsed: number;
  memTotal: number;
  memCache: number;
  diskUsed: number;
  diskTotal: number;
};

export type SandboxesMetricsResponse = {
  sandboxes: Record<string, SandboxMetric>;
};

export type SandboxRow = Sandbox & {
  metrics?: SandboxMetric;
};

export type Template = {
  templateID: string;
  buildStatus: string;
};

export type Volume = {
  volumeID: string;
  name: string;
};

export type APIError = {
  message: string;
  code: number;
};

export class PlatformAPIError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "PlatformAPIError";
    this.status = status;
  }
}
