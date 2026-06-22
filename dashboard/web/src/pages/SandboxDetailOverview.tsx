import { useState } from "react";
import { Link } from "react-router-dom";
import { DataTable, MarbleCard, type DataTableColumn } from "../components";
import type { SandboxVolumeMount } from "../api/types";
import { DetailField } from "./SandboxDetail";
import { useSandboxDetail } from "./sandboxDetailContext";
import { formatDateTime } from "../utils/sandbox";

export function SandboxDetailOverview() {
  const { sandbox } = useSandboxDetail();
  const [copied, setCopied] = useState(false);

  if (!sandbox) {
    return null;
  }

  const mountColumns: DataTableColumn<SandboxVolumeMount>[] = [
    { key: "name", header: "Volume", render: (row) => row.name },
    {
      key: "path",
      header: "Mount path",
      mono: true,
      render: (row) => row.path,
    },
  ];

  const mounts = sandbox.volumeMounts ?? [];

  return (
    <div className="sandbox-detail-grid">
      <MarbleCard title="Identity">
        <DetailField label="Sandbox ID">
          <span className="mono">{sandbox.sandboxID}</span>
          <button
            type="button"
            className="sandbox-detail-copy"
            onClick={() => {
              void navigator.clipboard.writeText(sandbox.sandboxID).then(() => {
                setCopied(true);
                window.setTimeout(() => setCopied(false), 1500);
              });
            }}
          >
            {copied ? "Copied" : "Copy"}
          </button>
        </DetailField>
        <DetailField label="Client">{sandbox.clientID}</DetailField>
        <DetailField label="Alias">{sandbox.alias || "—"}</DetailField>
        <DetailField label="Envd version">{sandbox.envdVersion}</DetailField>
      </MarbleCard>

      <MarbleCard title="Resources">
        <DetailField label="CPU">{sandbox.cpuCount} vCPU</DetailField>
        <DetailField label="Memory">{sandbox.memoryMB} MB</DetailField>
        <DetailField label="Disk">{sandbox.diskSizeMB} MB</DetailField>
        <DetailField label="Started">{formatDateTime(sandbox.startedAt)}</DetailField>
        <DetailField label="Ends">{formatDateTime(sandbox.endAt)}</DetailField>
      </MarbleCard>

      <MarbleCard title="Lifecycle">
        <DetailField label="On timeout">{sandbox.lifecycle.onTimeout}</DetailField>
        <DetailField label="Auto resume">
          {sandbox.lifecycle.autoResume ? "Enabled" : "Disabled"}
        </DetailField>
      </MarbleCard>

      <MarbleCard title="Network">
        <DetailField label="Domain">{sandbox.domain || "—"}</DetailField>
        <DetailField label="Internet access">
          {formatInternetAccess(sandbox.allowInternetAccess)}
        </DetailField>
        {sandbox.network ? (
          <>
            {sandbox.network.allowPublicTraffic !== undefined ? (
              <DetailField label="Public traffic">
                {sandbox.network.allowPublicTraffic ? "Allowed" : "Denied"}
              </DetailField>
            ) : null}
            {sandbox.network.denyOut?.length ? (
              <DetailField label="Deny out">
                <span className="mono">{sandbox.network.denyOut.join(", ")}</span>
              </DetailField>
            ) : null}
            {sandbox.network.allowOut?.length ? (
              <DetailField label="Allow out">
                <span className="mono">{sandbox.network.allowOut.join(", ")}</span>
              </DetailField>
            ) : null}
          </>
        ) : (
          <p className="sandbox-detail-muted">No custom network rules.</p>
        )}
      </MarbleCard>

      <MarbleCard title="Template" className="sandbox-detail-span-2">
        <DetailField label="Template ID">
          <Link to={`/templates/${sandbox.templateID}`} className="sandbox-detail-link">
            {sandbox.templateID}
          </Link>
        </DetailField>
      </MarbleCard>

      <MarbleCard title="Volume mounts" className="sandbox-detail-span-2">
        {mounts.length === 0 ? (
          <p className="sandbox-detail-muted">No volumes mounted.</p>
        ) : (
          <DataTable
            columns={mountColumns}
            rows={mounts}
            rowKey={(row) => `${row.name}:${row.path}`}
            emptyMessage="No volume mounts."
          />
        )}
      </MarbleCard>
    </div>
  );
}

function formatInternetAccess(value: boolean | null | undefined): string {
  if (value === undefined || value === null) {
    return "Default";
  }
  return value ? "Allowed" : "Denied";
}
