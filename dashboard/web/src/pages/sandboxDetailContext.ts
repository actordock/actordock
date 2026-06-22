import { createContext, useContext } from "react";
import type { SandboxDetail } from "../api/types";

type SandboxDetailContextValue = {
  sandbox: SandboxDetail | null;
  reload: () => void;
};

export const SandboxDetailContext = createContext<SandboxDetailContextValue>({
  sandbox: null,
  reload: () => undefined,
});

export function useSandboxDetail() {
  return useContext(SandboxDetailContext);
}
