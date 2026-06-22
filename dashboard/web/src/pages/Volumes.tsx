import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { fetchVolumes } from "../api/platform";
import type { Volume } from "../api/types";
import { DataTable, PageHeader, type DataTableColumn } from "../components";
import { useRefreshIntervalMs } from "../hooks/useRefreshInterval";
import { formatDateTime } from "../utils/sandbox";
import { filterVolumes, sortVolumes } from "../utils/volume";
import "./Volumes.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; rows: Volume[] }
  | { kind: "error"; message: string };

export function Volumes() {
  const navigate = useNavigate();
  const refreshMs = useRefreshIntervalMs();
  const [loadState, setLoadState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<"name" | "createdAt">("name");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const volumes = await fetchVolumes();
        if (!cancelled) {
          setLoadState({ kind: "ready", rows: volumes });
        }
      } catch (err) {
        if (!cancelled) {
          setLoadState({
            kind: "error",
            message: err instanceof Error ? err.message : "Failed to load volumes",
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

  const visibleRows = useMemo(() => {
    if (loadState.kind !== "ready") {
      return [];
    }
    return sortVolumes(filterVolumes(loadState.rows, search), sortKey, sortDir);
  }, [loadState, search, sortKey, sortDir]);

  const columns: DataTableColumn<Volume>[] = [
    {
      key: "name",
      header: "Name",
      render: (row) => row.name,
    },
    {
      key: "volumeID",
      header: "Volume ID",
      mono: true,
      render: (row) => row.volumeID,
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
        title="Volumes"
        subtitle="Persistent volume registry (read-only)."
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

      <p className="volumes-note">
        Runtime bind-mount into sandboxes is backlog (
        <a href="https://github.com/actordock/actordock/issues/48">#48</a> /{" "}
        <a href="https://github.com/actordock/actordock/issues/49">#49</a>
        ). Volumes can be registered and referenced at sandbox create time.
      </p>

      {loadState.kind === "error" ? (
        <div className="volumes-error" role="alert">
          {loadState.message}
        </div>
      ) : null}

      <section className="volumes-toolbar">
        <label className="volumes-field">
          <span>Search</span>
          <input
            type="search"
            placeholder="name or volume id"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>

        <label className="volumes-field">
          <span>Sort by</span>
          <select
            value={`${sortKey}:${sortDir}`}
            onChange={(e) => {
              const [key, dir] = e.target.value.split(":") as [
                typeof sortKey,
                typeof sortDir,
              ];
              setSortKey(key);
              setSortDir(dir);
            }}
          >
            <option value="name:asc">Name (A–Z)</option>
            <option value="name:desc">Name (Z–A)</option>
            <option value="createdAt:desc">Created (newest)</option>
            <option value="createdAt:asc">Created (oldest)</option>
          </select>
        </label>
      </section>

      {loadState.kind === "loading" ? (
        <p className="volumes-loading">Loading volumes…</p>
      ) : visibleRows.length === 0 ? (
        <div className="volumes-empty">
          <h3>
            {loadState.kind === "ready" && loadState.rows.length > 0
              ? "No volumes match search"
              : "No volumes yet"}
          </h3>
          <p>Create volumes via the Platform API or SDK.</p>
        </div>
      ) : (
        <DataTable
          columns={columns}
          rows={visibleRows}
          rowKey={(row) => row.volumeID}
          onRowClick={(row) => navigate(`/volumes/${row.volumeID}`)}
        />
      )}
    </>
  );
}
