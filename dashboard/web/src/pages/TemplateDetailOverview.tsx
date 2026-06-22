import { MarbleCard } from "../components";
import { formatDateTime } from "../utils/sandbox";
import { DetailField } from "./TemplateDetail";
import { useTemplateDetail } from "./templateDetailContext";

export function TemplateDetailOverview() {
  const { template } = useTemplateDetail();

  if (!template) {
    return null;
  }

  const latestBuild = template.builds[0];

  return (
    <div className="template-detail-grid">
      <MarbleCard title="Identity">
        <DetailField label="Template ID">{template.templateID}</DetailField>
        <DetailField label="Visibility">
          {template.public ? "Public" : "Private"}
        </DetailField>
        <DetailField label="Spawn count">{template.spawnCount}</DetailField>
        <DetailField label="Last spawned">
          {template.lastSpawnedAt
            ? formatDateTime(template.lastSpawnedAt)
            : "—"}
        </DetailField>
      </MarbleCard>

      <MarbleCard title="Timestamps">
        <DetailField label="Created">
          {formatDateTime(template.createdAt)}
        </DetailField>
        <DetailField label="Updated">
          {formatDateTime(template.updatedAt)}
        </DetailField>
        {latestBuild ? (
          <DetailField label="Latest build finished">
            {latestBuild.finishedAt
              ? formatDateTime(latestBuild.finishedAt)
              : "—"}
          </DetailField>
        ) : null}
      </MarbleCard>

      <MarbleCard title="Aliases" className="template-detail-span-2">
        {template.aliases.length === 0 ? (
          <p className="template-detail-muted">No aliases.</p>
        ) : (
          <div className="template-detail-chip-list">
            {template.aliases.map((alias) => (
              <span key={alias} className="template-detail-chip">
                {alias}
              </span>
            ))}
          </div>
        )}
      </MarbleCard>

      <MarbleCard title="Names" className="template-detail-span-2">
        {template.names.length === 0 ? (
          <p className="template-detail-muted">No additional names.</p>
        ) : (
          <div className="template-detail-chip-list">
            {template.names.map((name) => (
              <span key={name} className="template-detail-chip">
                {name}
              </span>
            ))}
          </div>
        )}
      </MarbleCard>

      {latestBuild ? (
        <MarbleCard title="Latest build resources" className="template-detail-span-2">
          <DetailField label="Build ID">
            <span className="mono">{latestBuild.buildID}</span>
          </DetailField>
          <DetailField label="Resources">
            {latestBuild.cpuCount} vCPU · {latestBuild.memoryMB} MB
            {latestBuild.diskSizeMB ? ` · ${latestBuild.diskSizeMB} MB disk` : ""}
          </DetailField>
          <DetailField label="Envd version">
            {latestBuild.envdVersion || "—"}
          </DetailField>
        </MarbleCard>
      ) : null}
    </div>
  );
}
