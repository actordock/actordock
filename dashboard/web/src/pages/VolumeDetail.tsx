import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useParams } from "react-router-dom";
import { fetchSandboxes, fetchVolume } from "../api/platform";
import type { Sandbox, VolumeDetail } from "../api/types";
import {
  DataTable,
  MarbleCard,
  StatusBadge,
  type DataTableColumn,
} from "../components";
import { formatDateTime, sandboxStatusVariant } from "../utils/sandbox";
import { sandboxesUsingVolume, type VolumeMountRef } from "../utils/volume";
import "./VolumeDetail.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; volume: VolumeDetail; sandboxes: Sandbox[] }
  | { kind: "error"; message: string };

export function VolumeDetail() {
  const { id = "" } = useParams();
  const [state, setState] = useState<LoadState>({ kind: "loading" });
  const [reloadToken, setReloadToken] = useState(0);
  const [tokenVisible, setTokenVisible] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setState({ kind: "loading" });
      try {
        const [volume, sandboxes] = await Promise.all([
          fetchVolume(id),
          fetchSandboxes().catch(() => [] as Sandbox[]),
        ]);
        if (!cancelled) {
          setState({ kind: "ready", volume, sandboxes });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message: err instanceof Error ? err.message : "Failed to load volume",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [id, reloadToken]);

  const mounts = useMemo(() => {
    if (state.kind !== "ready") {
      return [];
    }
    return sandboxesUsingVolume(state.sandboxes, state.volume);
  }, [state]);

  if (state.kind === "loading") {
    return <p className="volume-detail-loading">Loading volume…</p>;
  }

  if (state.kind === "error") {
    return (
      <div className="volume-detail-error" role="alert">
        <p>{state.message}</p>
        <Link to="/volumes" className="volume-detail-link">
          Back to volumes
        </Link>
      </div>
    );
  }

  const { volume } = state;
  const maskedToken = maskToken(volume.token);

  const mountColumns: DataTableColumn<VolumeMountRef>[] = [
    {
      key: "sandboxID",
      header: "Sandbox",
      mono: true,
      render: (row) => (
        <Link to={`/sandboxes/${row.sandboxID}`} className="volume-detail-link">
          {row.sandboxID}
        </Link>
      ),
    },
    {
      key: "template",
      header: "Template",
      render: (row) => row.templateID,
    },
    {
      key: "state",
      header: "State",
      render: (row) => (
        <StatusBadge variant={sandboxStatusVariant(row.state)} />
      ),
    },
    {
      key: "path",
      header: "Mount path",
      mono: true,
      render: (row) => row.mountPath,
    },
  ];

  return (
    <>
      <nav className="volume-detail-breadcrumb" aria-label="Breadcrumb">
        <Link to="/volumes">Volumes</Link>
        <span aria-hidden="true">/</span>
        <span>{volume.name}</span>
      </nav>

      <header className="volume-detail-header">
        <div>
          <h2 className="volume-detail-title">{volume.name}</h2>
          <p className="volume-detail-muted mono">{volume.volumeID}</p>
        </div>
        <button
          type="button"
          className="btn btn--ghost"
          onClick={() => setReloadToken((token) => token + 1)}
        >
          Refresh
        </button>
      </header>

      <p className="volume-detail-note">
        Runtime bind-mount is backlog (
        <a href="https://github.com/actordock/actordock/issues/48">#48</a> /{" "}
        <a href="https://github.com/actordock/actordock/issues/49">#49</a>
        ). Mount references below come from sandbox metadata only.
      </p>

      <div className="volume-detail-grid">
        <MarbleCard title="Identity">
          <DetailField label="Name">{volume.name}</DetailField>
          <DetailField label="Volume ID">
            <span className="mono">{volume.volumeID}</span>
          </DetailField>
          <DetailField label="Created">
            {formatDateTime(volume.createdAt ?? "")}
          </DetailField>
          {volume.hostPath ? (
            <DetailField label="Host path">
              <span className="mono">{volume.hostPath}</span>
            </DetailField>
          ) : null}
        </MarbleCard>

        <MarbleCard title="Access token">
          <div className="volume-detail-token">
            <code className="mono">
              {tokenVisible ? volume.token : maskedToken}
            </code>
            <div className="volume-detail-token-actions">
              <button
                type="button"
                className="btn btn--ghost"
                onClick={() => setTokenVisible((value) => !value)}
              >
                {tokenVisible ? "Hide" : "Reveal"}
              </button>
              <button
                type="button"
                className="btn btn--ghost"
                onClick={() => {
                  void navigator.clipboard.writeText(volume.token).then(() => {
                    setCopied(true);
                    window.setTimeout(() => setCopied(false), 1500);
                  });
                }}
              >
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
          </div>
          <p className="volume-detail-muted">
            Token is required when mounting this volume at sandbox create time.
          </p>
        </MarbleCard>

        <MarbleCard title="Mounted by" className="volume-detail-span-2">
          {mounts.length === 0 ? (
            <p className="volume-detail-muted">
              No running sandboxes reference this volume in their mount list.
            </p>
          ) : (
            <DataTable
              columns={mountColumns}
              rows={mounts}
              rowKey={(row) => `${row.sandboxID}:${row.mountPath}`}
              emptyMessage="No sandboxes mount this volume."
            />
          )}
        </MarbleCard>
      </div>
    </>
  );
}

function DetailField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="volume-detail-field">
      <span className="volume-detail-field__label">{label}</span>
      <span className="volume-detail-field__value">{children}</span>
    </div>
  );
}

function maskToken(token: string): string {
  if (token.length <= 8) {
    return "••••••••";
  }
  return `${token.slice(0, 4)}••••${token.slice(-4)}`;
}
