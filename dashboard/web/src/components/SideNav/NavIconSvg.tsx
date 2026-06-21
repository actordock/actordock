import type { NavIcon } from "../../routes/nav";

type NavIconProps = {
  name: NavIcon;
  className?: string;
};

export function NavIconSvg({ name, className }: NavIconProps) {
  const props = {
    className,
    width: 18,
    height: 18,
    viewBox: "0 0 18 18",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.25,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };

  switch (name) {
    case "overview":
      return (
        <svg {...props}>
          <rect x="2" y="2" width="6" height="6" rx="1" />
          <rect x="10" y="2" width="6" height="6" rx="1" />
          <rect x="2" y="10" width="6" height="6" rx="1" />
          <rect x="10" y="10" width="6" height="6" rx="1" />
        </svg>
      );
    case "sandboxes":
      return (
        <svg {...props}>
          <rect x="3" y="5" width="12" height="9" rx="1.5" />
          <path d="M6 5V3.5A1.5 1.5 0 0 1 7.5 2h3A1.5 1.5 0 0 1 12 3.5V5" />
        </svg>
      );
    case "templates":
      return (
        <svg {...props}>
          <path d="M4 3h10v12H4z" />
          <path d="M7 7h4M7 10h4" />
        </svg>
      );
    case "volumes":
      return (
        <svg {...props}>
          <ellipse cx="9" cy="5" rx="5" ry="2" />
          <path d="M4 5v8c0 1.1 2.24 2 5 2s5-.9 5-2V5" />
        </svg>
      );
    case "snapshots":
      return (
        <svg {...props}>
          <circle cx="9" cy="9" r="6" />
          <circle cx="9" cy="9" r="2.5" />
        </svg>
      );
    case "monitoring":
      return (
        <svg {...props}>
          <path d="M2 14l4-5 3 3 5-8 2 3" />
        </svg>
      );
    case "settings":
      return (
        <svg {...props}>
          <circle cx="9" cy="9" r="2.5" />
          <path d="M9 1.5v2M9 14.5v2M1.5 9h2M14.5 9h2M3.5 3.5l1.4 1.4M13.1 13.1l1.4 1.4M3.5 14.5l1.4-1.4M13.1 4.9l1.4-1.4" />
        </svg>
      );
    case "theme":
      return (
        <svg {...props}>
          <path d="M9 2v14M4 7h10M5 12h8" />
          <ellipse cx="9" cy="4" rx="3" ry="1.5" />
        </svg>
      );
  }
}
