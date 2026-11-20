import { useSyncExternalStore } from "react";
import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Theme = "light" | "dark" | "system";
export type EditorMode = "split" | "edit" | "preview";

interface UIState {
  theme: Theme;
  sidebarOpen: boolean;
  editorMode: EditorMode;
  vim: boolean;
  /** Mobile navigation drawer (not persisted). */
  mobileNavOpen: boolean;
  setMobileNavOpen: (open: boolean) => void;
  setTheme: (t: Theme) => void;
  setSidebarOpen: (open: boolean) => void;
  setEditorMode: (m: EditorMode) => void;
  setVim: (v: boolean) => void;
}

export const useUI = create<UIState>()(
  persist(
    (set) => ({
      theme: "system",
      sidebarOpen: true,
      editorMode: "split",
      vim: false,
      mobileNavOpen: false,
      setMobileNavOpen: (mobileNavOpen) => set({ mobileNavOpen }),
      setTheme: (theme) => {
        // public/theme-init.js reads this key before the app loads.
        localStorage.setItem("theme", theme);
        set({ theme });
      },
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      setEditorMode: (editorMode) => set({ editorMode }),
      setVim: (vim) => set({ vim }),
    }),
    {
      name: "ui",
      partialize: (s) => ({ theme: s.theme, sidebarOpen: s.sidebarOpen, editorMode: s.editorMode, vim: s.vim }),
    },
  ),
);

const media = window.matchMedia("(prefers-color-scheme: dark)");

function applyTheme(theme: Theme) {
  const dark = theme === "dark" || (theme === "system" && media.matches);
  document.documentElement.classList.toggle("dark", dark);
}

applyTheme(useUI.getState().theme);
useUI.subscribe((s) => applyTheme(s.theme));
media.addEventListener("change", () => applyTheme(useUI.getState().theme));

function subscribeDark(cb: () => void) {
  const obs = new MutationObserver(cb);
  obs.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => obs.disconnect();
}

/** Whether dark mode is currently active. */
export function useIsDark() {
  return useSyncExternalStore(subscribeDark, () => document.documentElement.classList.contains("dark"));
}
