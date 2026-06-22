import { useEffect, useState } from "react";
import { fetchSandboxLogsV2 } from "../api/platform";
import type { SandboxLogEntry } from "../api/types";
import { VirtualList } from "../components/VirtualList/VirtualList";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import {
  downloadLogs,
  filterLogEntries,
  logStream,
  type LogFilters,
} from "../utils/sandboxLogs";
import { useSandboxDetail } from "./sandboxDetailContext";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; entries: SandboxLogEntry[] }
  | { kind: "error"; message: string };

const LOG_ROW_HEIGHT = 72;
const LOG_VIEWPORT_HEIGHT = 420;
const defaultFilters: LogFilters = { level: "", search: "" };

export function SandboxDetailLogs() {
  const { sandbox } = useSandboxDetail();
  const refreshMs = useRefreshIntervalMs();
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const [filters, setFilters] = useState<LogFilters>(defaultFilters);
  const [apiFilters, setApiFilters] = useState<LogFilters>(defaultFilters);
  const [tail, setTail] = useState(true);
  const [paused, setPaused] = useState(false);
  const [nearBottom, setNearBottom] = useState(true);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    if (!sandbox) {
      return;
    }
    const sandboxID = sandbox.sandboxID;
    let cancelled = false;

    async function load() {
      try {
        const entries = await fetchSandboxLogsV2(sandboxID, {
          limit: 1000,
          level: apiFilters.level || undefined,
          search: apiFilters.search || undefined,
        });
        if (!cancelled) {
          setState({ kind: "ready", entries });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message: err instanceof Error ? err.message : "Failed to load logs",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [sandbox, reloadToken, apiFilters]);

  useEffect(() => {
    if (!sandbox || refreshMs <= 0) {
      return;
    }
    const id = window.setInterval(() => {
      setReloadToken((token) => token + 1);
    }, refreshMs);
    return () => window.clearInterval(id);
  }, [sandbox, refreshMs]);

  if (!sandbox) {
    return null;
  }

  const visibleEntries =
    state.kind === "ready"
      ? filterLogEntries(state.entries, filters)
      : [];

  const shouldAutoScroll = tail && !paused && nearBottom;

  return (
    <>
      <div className="sandbox-detail-logs-toolbar">
        <label className="sandbox-detail-field">
          <span>Level</span>
          <select
            value={filters.level}
            onChange={(e) =>
              setFilters((prev) => ({ ...prev, level: e.target.value }))
            }
          >
            <option value="">All levels</option>
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>
        </label>

        <label className="sandbox-detail-field sandbox-detail-field--grow">
          <span>Search</span>
          <input
            type="search"
            placeholder="Filter message or stream"
            value={filters.search}
            onChange={(e) =>
              setFilters((prev) => ({ ...prev, search: e.target.value }))
            }
          />
        </label>

        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => setApiFilters({ ...filters })}
        >
          Apply server filter
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => setReloadToken((token) => token + 1)}
        >
          Refresh
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => {
            setPaused((value) => !value);
            if (paused) {
              setTail(true);
            }
          }}
        >
          {paused ? "Resume tail" : "Pause tail"}
        </button>
        <label className="sandbox-detail-checkbox">
          <input
            type="checkbox"
            checked={tail}
            onChange={(e) => setTail(e.target.checked)}
          />
          Auto-scroll
        </label>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={visibleEntries.length === 0}
          onClick={() => downloadLogs(visibleEntries, sandbox.sandboxID, "txt")}
        >
          Export .txt
        </button>
        <button
          type="button"
          className="btn btn--ghost"
          disabled={visibleEntries.length === 0}
          onClick={() => downloadLogs(visibleEntries, sandbox.sandboxID, "jsonl")}
        >
          Export .jsonl
        </button>
      </div>

      {state.kind === "loading" ? (
        <p className="sandbox-detail-muted">Loading logs…</p>
      ) : state.kind === "error" ? (
        <div className="sandbox-detail-error" role="alert">
          {state.message}
        </div>
      ) : visibleEntries.length === 0 ? (
        <p className="sandbox-detail-muted">No log entries match the current filters.</p>
      ) : (
        <VirtualList
          items={visibleEntries}
          rowHeight={LOG_ROW_HEIGHT}
          height={LOG_VIEWPORT_HEIGHT}
          scrollToBottom={shouldAutoScroll}
          onScroll={(scrollTop) => {
            const maxScroll =
              visibleEntries.length * LOG_ROW_HEIGHT - LOG_VIEWPORT_HEIGHT;
            const atBottom = scrollTop >= maxScroll - LOG_ROW_HEIGHT;
            setNearBottom(atBottom);
            if (!atBottom && tail && !paused) {
              setPaused(true);
            }
          }}
          rowKey={(entry, index) => `${entry.timestamp}:${index}`}
          renderRow={(entry) => <LogRow entry={entry} />}
        />
      )}
    </>
  );
}

function LogRow({ entry }: { entry: SandboxLogEntry }) {
  const stream = logStream(entry);
  return (
    <div className="log-row">
      <div className="log-row__meta">
        <time className="log-row__time mono">{entry.timestamp}</time>
        <span className={`log-row__level log-row__level--${entry.level}`}>
          {entry.level}
        </span>
        {stream ? <span className="log-row__stream mono">{stream}</span> : null}
      </div>
      <pre className="log-row__message mono">{entry.message}</pre>
    </div>
  );
}
