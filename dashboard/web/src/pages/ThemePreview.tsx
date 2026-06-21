import { useState } from "react";
import {
  DataTable,
  MarbleCard,
  PageHeader,
  StatusBadge,
  type DataTableColumn,
} from "../components";
import "./ThemePreview.css";

type SampleRow = {
  id: string;
  template: string;
  state: "running" | "paused" | "failed";
  startedAt: string;
};

const sampleRows: SampleRow[] = [
  {
    id: "sbx_7f3a9c2e1b",
    template: "python-3.12",
    state: "running",
    startedAt: "2026-06-21 09:14",
  },
  {
    id: "sbx_2d8e4f1a0c",
    template: "node-20",
    state: "paused",
    startedAt: "2026-06-21 08:02",
  },
  {
    id: "sbx_9b1c5e7d3f",
    template: "base",
    state: "failed",
    startedAt: "2026-06-20 22:41",
  },
];

const columns: DataTableColumn<SampleRow>[] = [
  {
    key: "id",
    header: "Sandbox ID",
    mono: true,
    render: (row) => row.id,
  },
  {
    key: "template",
    header: "Template",
    render: (row) => row.template,
  },
  {
    key: "state",
    header: "State",
    render: (row) => <StatusBadge variant={row.state} />,
  },
  {
    key: "startedAt",
    header: "Started",
    render: (row) => row.startedAt,
  },
];

const swatches = [
  { name: "marble-base", var: "--marble-base" },
  { name: "marble-card", var: "--marble-card" },
  { name: "marble-shadow", var: "--marble-shadow" },
  { name: "stone-ink", var: "--stone-ink" },
  { name: "stone-muted", var: "--stone-muted" },
  { name: "bronze-accent", var: "--bronze-accent" },
  { name: "bronze-deep", var: "--bronze-deep" },
  { name: "laurel-ok", var: "--laurel-ok" },
  { name: "amber-pause", var: "--amber-pause" },
  { name: "terracotta-danger", var: "--terracotta-danger" },
];

export function ThemePreview() {
  const [metric, setMetric] = useState(42);

  return (
    <>
      <PageHeader
        title="Theme Preview"
        subtitle="Greek marble design system — tokens, components, and motion QA."
      />

      <section className="theme-preview__section">
        <h3 className="theme-preview__heading">Color tokens</h3>
        <div className="theme-preview__swatches">
          {swatches.map((s) => (
            <div key={s.name} className="theme-preview__swatch">
              <span
                className="theme-preview__swatch-color"
                style={{ background: `var(${s.var})` }}
              />
              <span className="theme-preview__swatch-name">{s.name}</span>
            </div>
          ))}
        </div>
      </section>

      <section className="theme-preview__section">
        <h3 className="theme-preview__heading">Typography</h3>
        <MarbleCard title="Type scale">
          <p className="theme-preview__display">Actordock — Display</p>
          <p className="theme-preview__body">
            Body text uses Inter for UI clarity across tables, forms, and
            navigation labels.
          </p>
          <p className="theme-preview__mono mono">
            sbx_7f3a9c2e1b — monospace for IDs and log lines
          </p>
        </MarbleCard>
      </section>

      <section className="theme-preview__section">
        <h3 className="theme-preview__heading">Status badges</h3>
        <div className="theme-preview__badges">
          <StatusBadge variant="running" />
          <StatusBadge variant="paused" />
          <StatusBadge variant="failed" />
          <StatusBadge variant="starting" />
          <StatusBadge variant="killed" />
          <StatusBadge variant="unknown" />
        </div>
      </section>

      <section className="theme-preview__section">
        <h3 className="theme-preview__heading">Exhibit cards</h3>
        <div className="theme-preview__cards">
          <MarbleCard title="Metric">
            <div className="theme-preview__metric">
              <span className="theme-preview__metric-value">{metric}</span>
              <span className="theme-preview__metric-label">active sandboxes</span>
            </div>
          </MarbleCard>
          <MarbleCard
            title="Actions"
            footer="Primary actions use bronze accent on marble surfaces."
          >
            <div className="theme-preview__actions">
              <button type="button" className="btn btn--primary">
                Primary
              </button>
              <button type="button" className="btn btn--ghost">
                Secondary
              </button>
              <button
                type="button"
                className="btn btn--ghost"
                onClick={() => {
                  setMetric((v) => v + 1);
                  const el = document.querySelector(".theme-preview__metric-value");
                  el?.classList.remove("shimmer");
                  void (el as HTMLElement)?.offsetWidth;
                  el?.classList.add("shimmer");
                }}
              >
                Shimmer refresh
              </button>
            </div>
          </MarbleCard>
        </div>
      </section>

      <section className="theme-preview__section">
        <h3 className="theme-preview__heading">Data table</h3>
        <DataTable
          columns={columns}
          rows={sampleRows}
          rowKey={(row) => row.id}
          emptyMessage="No sandboxes yet."
        />
      </section>
    </>
  );
}
