import { createContext, useContext } from "react";
import type { TemplateDetail } from "../api/types";

type TemplateDetailContextValue = {
  template: TemplateDetail | null;
  reload: () => void;
};

export const TemplateDetailContext = createContext<TemplateDetailContextValue>({
  template: null,
  reload: () => undefined,
});

export function useTemplateDetail() {
  return useContext(TemplateDetailContext);
}
