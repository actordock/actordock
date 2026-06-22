import type { StatusVariant } from "../components/StatusBadge/StatusBadge";
import type { Template } from "../api/types";

export function buildStatusVariant(status: string): StatusVariant {
  switch (status.toLowerCase()) {
    case "ready":
    case "success":
      return "running";
    case "building":
    case "pending":
      return "starting";
    case "failed":
      return "failed";
    default:
      return "unknown";
  }
}

export function formatTemplateResources(template: Template): string {
  return `${template.cpuCount} vCPU · ${template.memoryMB} MB · ${template.diskSizeMB} MB disk`;
}

export function filterTemplates(
  templates: Template[],
  search: string,
): Template[] {
  const query = search.trim().toLowerCase();
  if (!query) {
    return templates;
  }
  return templates.filter((template) => {
    const haystack = [
      template.templateID,
      template.buildStatus,
      template.envdVersion,
      ...(template.aliases ?? []),
      ...(template.names ?? []),
    ]
      .join(" ")
      .toLowerCase();
    return haystack.includes(query);
  });
}

export function sortTemplates(
  templates: Template[],
  sortKey: "templateID" | "buildStatus" | "createdAt",
  sortDir: "asc" | "desc",
): Template[] {
  const sign = sortDir === "asc" ? 1 : -1;
  return [...templates].sort((a, b) => {
    switch (sortKey) {
      case "buildStatus":
        return sign * a.buildStatus.localeCompare(b.buildStatus);
      case "createdAt":
        return sign * (dateValue(a.createdAt) - dateValue(b.createdAt));
      case "templateID":
      default:
        return sign * a.templateID.localeCompare(b.templateID);
    }
  });
}

function dateValue(iso?: string): number {
  if (!iso) {
    return 0;
  }
  const value = new Date(iso).getTime();
  return Number.isNaN(value) ? 0 : value;
}
