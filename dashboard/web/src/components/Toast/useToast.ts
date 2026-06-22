import { useContext } from "react";
import { ToastContext } from "./toastContext";

export function useToast() {
  return useContext(ToastContext);
}
