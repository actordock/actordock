import { useEffect, useState } from "react";
import { fetchTemplateTags } from "../api/platform";
import type { TemplateTag } from "../api/types";
import { DataTable, type DataTableColumn } from "../components";
import { formatDateTime } from "../utils/sandbox";
import { useTemplateDetail } from "./templateDetailContext";

type LoadState =
  | { kind: "loading" }
  | { kind: "ready"; tags: TemplateTag[] }
  | { kind: "error"; message: string };

export function TemplateDetailTags() {
  const { template } = useTemplateDetail();
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  useEffect(() => {
    if (!template) {
      return;
    }
    const templateID = template.templateID;
    let cancelled = false;

    async function load() {
      try {
        const tags = await fetchTemplateTags(templateID);
        if (!cancelled) {
          setState({ kind: "ready", tags });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message: err instanceof Error ? err.message : "Failed to load tags",
          });
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [template]);

  if (!template) {
    return null;
  }

  const columns: DataTableColumn<TemplateTag>[] = [
    { key: "tag", header: "Tag", render: (row) => row.tag },
    {
      key: "buildID",
      header: "Build ID",
      mono: true,
      render: (row) => row.buildID,
    },
    {
      key: "createdAt",
      header: "Created",
      render: (row) => formatDateTime(row.createdAt),
    },
  ];

  if (state.kind === "loading") {
    return <p className="template-detail-muted">Loading tags…</p>;
  }

  if (state.kind === "error") {
    return (
      <div className="template-detail-error" role="alert">
        {state.message}
      </div>
    );
  }

  return (
    <DataTable
      columns={columns}
      rows={state.tags}
      rowKey={(row) => `${row.tag}:${row.buildID}`}
      emptyMessage="No tags defined for this template."
    />
  );
}
