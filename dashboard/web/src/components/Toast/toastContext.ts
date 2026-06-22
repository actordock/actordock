import { createContext } from "react";

export type ToastTone = "info" | "success" | "error";

export type ToastContextValue = {
  pushToast: (message: string, tone?: ToastTone) => void;
};

export const ToastContext = createContext<ToastContextValue>({
  pushToast: () => undefined,
});
