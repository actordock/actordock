import { useEffect, useState } from "react";
import { loadPrefs, prefsEventName } from "../settings/prefs";

export function useRefreshIntervalMs(): number {
  const [intervalMs, setIntervalMs] = useState(() => {
    const sec = loadPrefs().refreshIntervalSec;
    return sec > 0 ? sec * 1000 : 0;
  });

  useEffect(() => {
    const sync = () => {
      const sec = loadPrefs().refreshIntervalSec;
      setIntervalMs(sec > 0 ? sec * 1000 : 0);
    };
    window.addEventListener(prefsEventName(), sync);
    return () => window.removeEventListener(prefsEventName(), sync);
  }, []);

  return intervalMs;
}
