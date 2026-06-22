import type { SandboxLogEntry } from "../api/types";

export type LogFilters = {
  level: string;
  search: string;
};

export function filterLogEntries(
  entries: SandboxLogEntry[],
  filters: LogFilters,
): SandboxLogEntry[] {
  const search = filters.search.trim().toLowerCase();
  return entries.filter((entry) => {
    if (filters.level && entry.level.toLowerCase() !== filters.level) {
      return false;
    }
    if (!search) {
      return true;
    }
    const haystack = [
      entry.message,
      entry.level,
      entry.timestamp,
      logStream(entry),
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(search);
  });
}

export function logStream(entry: SandboxLogEntry): string {
  return entry.fields?.stream ?? "";
}

export function downloadLogs(
  entries: SandboxLogEntry[],
  sandboxID: string,
  format: "txt" | "jsonl",
) {
  let content: string;
  let mime: string;
  let ext: string;

  if (format === "jsonl") {
    content = entries.map((entry) => JSON.stringify(entry)).join("\n");
    mime = "application/x-ndjson";
    ext = "jsonl";
  } else {
    content = entries
      .map(
        (entry) =>
          `${entry.timestamp} [${entry.level}] ${logStream(entry) ? `(${logStream(entry)}) ` : ""}${entry.message}`,
      )
      .join("\n");
    mime = "text/plain";
    ext = "txt";
  }

  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${sandboxID}-logs.${ext}`;
  anchor.click();
  URL.revokeObjectURL(url);
}
