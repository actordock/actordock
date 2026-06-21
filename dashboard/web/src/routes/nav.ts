export type NavItem = {
  label: string;
  path: string;
  icon: NavIcon;
};

export type NavIcon =
  | "overview"
  | "sandboxes"
  | "templates"
  | "volumes"
  | "snapshots"
  | "monitoring"
  | "settings"
  | "theme";

export const navItems: NavItem[] = [
  { label: "Overview", path: "/", icon: "overview" },
  { label: "Sandboxes", path: "/sandboxes", icon: "sandboxes" },
  { label: "Templates", path: "/templates", icon: "templates" },
  { label: "Volumes", path: "/volumes", icon: "volumes" },
  { label: "Snapshots", path: "/snapshots", icon: "snapshots" },
  { label: "Monitoring", path: "/monitoring", icon: "monitoring" },
  { label: "Settings", path: "/settings", icon: "settings" },
  { label: "Theme Preview", path: "/theme-preview", icon: "theme" },
];
