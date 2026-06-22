export type ThemeDensity = "comfortable" | "compact";

export type DashboardPrefs = {
  platformURL: string;
  apiKey: string;
  refreshIntervalSec: number;
  logTailLines: number;
  themeDensity: ThemeDensity;
};

const STORAGE_KEY = "actordock.settings";

export const defaultPrefs: DashboardPrefs = {
  platformURL: "",
  apiKey: "",
  refreshIntervalSec: 30,
  logTailLines: 200,
  themeDensity: "comfortable",
};

export function loadPrefs(): DashboardPrefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return { ...defaultPrefs };
    }
    const parsed = JSON.parse(raw) as Partial<DashboardPrefs>;
    return {
      platformURL: parsed.platformURL ?? defaultPrefs.platformURL,
      apiKey: parsed.apiKey ?? defaultPrefs.apiKey,
      refreshIntervalSec:
        parsed.refreshIntervalSec ?? defaultPrefs.refreshIntervalSec,
      logTailLines: parsed.logTailLines ?? defaultPrefs.logTailLines,
      themeDensity: parsed.themeDensity ?? defaultPrefs.themeDensity,
    };
  } catch {
    return { ...defaultPrefs };
  }
}

export function savePrefs(prefs: DashboardPrefs): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
}

export function prefsEventName(): string {
  return "actordock:prefs-updated";
}

export function notifyPrefsUpdated(): void {
  window.dispatchEvent(new Event(prefsEventName()));
}
