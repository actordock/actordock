import "./StatusBadge.css";

export type StatusVariant =
  | "running"
  | "paused"
  | "failed"
  | "starting"
  | "killed"
  | "unknown";

type StatusBadgeProps = {
  variant: StatusVariant;
  label?: string;
};

const defaultLabels: Record<StatusVariant, string> = {
  running: "Running",
  paused: "Paused",
  failed: "Failed",
  starting: "Starting",
  killed: "Killed",
  unknown: "Unknown",
};

function StatusIcon({ variant }: { variant: StatusVariant }) {
  const props = {
    width: 14,
    height: 14,
    viewBox: "0 0 14 14",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.1,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true as const,
  };

  switch (variant) {
    case "running":
      return (
        <svg {...props}>
          <path d="M7 1v3M5 2.5h4" />
          <path d="M4 5c0-1.5 1.3-2.5 3-2.5s3 1 3 2.5v5c0 1.5-1.3 2.5-3 2.5s-3-1-3-2.5V5z" />
          <path d="M6 9.5h2" />
        </svg>
      );
    case "paused":
      return (
        <svg {...props}>
          <circle cx="7" cy="5" r="2.5" />
          <path d="M4 10c.5-1.5 1.8-2.5 3-2.5s2.5 1 3 2.5" />
          <path d="M5.5 12.5h3" />
        </svg>
      );
    case "failed":
      return (
        <svg {...props}>
          <path d="M5 2v8M9 2v5" />
          <path d="M3 11h8" />
          <path d="M4 2h1M9 2h1" />
        </svg>
      );
    case "starting":
      return (
        <svg {...props}>
          <circle cx="7" cy="7" r="4.5" />
          <path d="M7 4v3l2 1.5" />
        </svg>
      );
    case "killed":
      return (
        <svg {...props}>
          <path d="M3 3l8 8M11 3l-8 8" />
        </svg>
      );
    default:
      return (
        <svg {...props}>
          <circle cx="7" cy="7" r="4.5" />
          <path d="M7 5v2.5" />
        </svg>
      );
  }
}

export function StatusBadge({ variant, label }: StatusBadgeProps) {
  const text = label ?? defaultLabels[variant];

  return (
    <span className={`status-badge status-badge--${variant}`}>
      <StatusIcon variant={variant} />
      {text}
    </span>
  );
}
