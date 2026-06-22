import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { fetchSandboxes, fetchSnapshots } from "../api/platform";
import type { Sandbox, Snapshot } from "../api/types";
import { DataTable, PageHeader, type DataTableColumn } from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import { formatDateTime } from "../utils/sandbox";
import {
  filterSnapshots,
  snapshotDisplayName,
  sortSnapshots,
} from "../utils/snapshot";
import "./Snapshots.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; snapshots: Snapshot[]; sandboxIds: Set<string> }
  | { kind: "error"; message: string };

type SnapshotRow = Snapshot & {
  sandboxActive: boolean;
};

export function Snapshots() {
  const navigate = useNavigate();
  const refreshMs = useRefreshIntervalMs();
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [search, setSearch] = useState("");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const [snapshots, sandboxes] = await Promise.all([
          fetchSnapshots(),
          fetchSandboxes().catch(() => [] as Sandbox[]),
        ]);
        if (!cancelled) {
          setLoadState({
            kind: "ready",
            snapshots,
            sandboxIds: new Set(sandboxes.map((sb) => sb.sandboxID)),
          });
        }
      } catch (err) {
        if (!cancelled) {
          setLoadState({
            kind: "error",
            message:
              err instanceof Error ? err.message : "Failed to load snapshots",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  useEffect(() => {
    if (refreshMs <= 0) {
      return;
    }
    const id = window.setInterval(() => {
      setReloadToken((token) => token + 1);
    }, refreshMs);
    return () => window.clearInterval(id);
  }, [refreshMs]);

  const visibleRows = useMemo((): SnapshotRow[] => {
    if (loadState.kind !== "ready") {
      return [];
    }
    const filtered = filterSnapshots(loadState.snapshots, search);
    const sorted = sortSnapshots(filtered, sortDir);
    return sorted.map((snapshot) => ({
      ...snapshot,
      sandboxActive: snapshot.sandboxID
        ? loadState.sandboxIds.has(snapshot.sandboxID)
        : false,
    }));
  }, [loadState, search, sortDir]);

  const columns: DataTableColumn<SnapshotRow>[] = [
    {
      key: "snapshotID",
      header: "Snapshot",
      mono: true,
      render: (row) => snapshotDisplayName(row),
    },
    {
      key: "sandboxID",
      header: "Sandbox",
      mono: true,
      render: (row) => {
        if (!row.sandboxID) {
          return "—";
        }
        if (row.sandboxActive) {
          return (
            <Link to={`/sandboxes/${row.sandboxID}`} className="snapshots-link">
              {row.sandboxID}
            </Link>
          );
        }
        return (
          <span className="snapshots-orphan" title="Sandbox no longer listed">
            {row.sandboxID}
          </span>
        );
      },
    },
    {
      key: "names",
      header: "Names",
      render: (row) => row.names.join(", ") || "—",
    },
    {
      key: "createdAt",
      header: "Created",
      render: (row) => formatDateTime(row.createdAt ?? ""),
    },
  ];

  return (
    <>
      <PageHeader
        title="Snapshots"
        subtitle="Sandbox snapshot history (read-only)."
        actions={
          <button
            type="button"
            className="btn btn--ghost"
            onClick={() => setReloadToken((token) => token + 1)}
          >
            Refresh
          </button>
        }
      />

      <p className="snapshots-note">
        Create snapshot from a sandbox is deferred to WP11. Orphaned rows keep
        snapshot metadata when the source sandbox is gone.
      </p>

      {loadState.kind === "error" ? (
        <div className="snapshots-error" role="alert">
          {loadState.message}
        </div>
      ) : null}

      <section className="snapshots-toolbar">
        <label className="snapshots-field">
          <span>Search</span>
          <input
            type="search"
            placeholder="snapshot id, sandbox id, name"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>

        <label className="snapshots-field">
          <span>Sort by</span>
          <select
            value={sortDir}
            onChange={(e) => setSortDir(e.target.value as typeof sortDir)}
          >
            <option value="desc">Created (newest)</option>
            <option value="asc">Created (oldest)</option>
          </select>
        </label>
      </section>

      {loadState.kind === "loading" ? (
        <p className="snapshots-loading">Loading snapshots…</p>
      ) : visibleRows.length === 0 ? (
        <div className="snapshots-empty">
          <h3>
            {loadState.kind === "ready" && loadState.snapshots.length > 0
              ? "No snapshots match search"
              : "No snapshots yet"}
          </h3>
          <p>Snapshots appear after POST /sandboxes/:id/snapshots.</p>
        </div>
      ) : (
        <DataTable
          columns={columns}
          rows={visibleRows}
          rowKey={(row) => row.snapshotID}
          onRowClick={(row) => {
            if (row.sandboxID && row.sandboxActive) {
              navigate(`/sandboxes/${row.sandboxID}`);
            }
          }}
        />
      )}
    </>
  );
}
