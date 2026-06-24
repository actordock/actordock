import type { TemplateBuild } from "../api/types";
import { DataTable, StatusBadge, type DataTableColumn } from "../components";
import { formatDateTime } from "../utils/sandbox";
import { buildStatusVariant } from "../utils/template";
import { useTemplateDetail } from "./templateDetailContext";

export function TemplateDetailBuilds() {
  const { template } = useTemplateDetail();

  if (!template) {
    return null;
  }

  const columns: DataTableColumn<TemplateBuild>[] = [
    {
      key: "buildID",
      header: "Build ID",
      mono: true,
      render: (row) => row.buildID,
    },
    {
      key: "status",
      header: "Status",
      render: (row) => (
        <StatusBadge
          variant={buildStatusVariant(row.status)}
          label={row.status}
        />
      ),
    },
    {
      key: "resources",
      header: "Resources",
      render: (row) =>
        `${row.cpuCount} vCPU · ${row.memoryMB} MB${row.diskSizeMB ? ` · ${row.diskSizeMB} MB disk` : ""}`,
    },
    {
      key: "envdVersion",
      header: "Envd",
      render: (row) => row.envdVersion || "—",
    },
    {
      key: "createdAt",
      header: "Created",
      render: (row) => formatDateTime(row.createdAt),
    },
    {
      key: "finishedAt",
      header: "Finished",
      render: (row) =>
        row.finishedAt ? formatDateTime(row.finishedAt) : "—",
    },
  ];

  return (
    <>
      <p className="template-detail-muted template-detail-builds-note">
        Build history is read-only. Upload and rebuild flows are not available
        yet.
      </p>
      <DataTable
        columns={columns}
        rows={template.builds}
        rowKey={(row) => row.buildID}
        emptyMessage="No builds recorded for this template."
      />
    </>
  );
}
