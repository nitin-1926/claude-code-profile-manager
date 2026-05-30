"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

type AiSearchContextValue = {
  open: boolean;
  openDialog: () => void;
  closeDialog: () => void;
};

const AiSearchContext = createContext<AiSearchContextValue | null>(null);

export function AiSearchProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);

  const openDialog = useCallback(() => setOpen(true), []);
  const closeDialog = useCallback(() => setOpen(false), []);

  const value = useMemo(
    () => ({ open, openDialog, closeDialog }),
    [open, openDialog, closeDialog],
  );

  return (
    <AiSearchContext.Provider value={value}>
      {children}
    </AiSearchContext.Provider>
  );
}

export function useAiSearch() {
  const ctx = useContext(AiSearchContext);
  if (!ctx) {
    throw new Error("useAiSearch must be used within AiSearchProvider");
  }
  return ctx;
}
