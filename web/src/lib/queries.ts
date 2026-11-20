import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken } from "./api";

export const keys = {
  me: ["me"] as const,
  site: ["site"] as const,
  notebooks: ["notebooks"] as const,
  tags: ["tags"] as const,
  notes: ["notes"] as const,
  note: (id: number) => ["note", id] as const,
  stats: ["stats"] as const,
  settings: ["settings"] as const,
  public: ["public"] as const,
};

export function useMe() {
  return useQuery({
    queryKey: keys.me,
    queryFn: async () => {
      const me = await api.me();
      setCsrfToken(me.csrfToken);
      return me;
    },
    staleTime: 5 * 60_000,
  });
}

export function useSite() {
  return useQuery({ queryKey: keys.site, queryFn: api.site, staleTime: 5 * 60_000 });
}

export function useNotebooks() {
  return useQuery({ queryKey: keys.notebooks, queryFn: api.notebooks });
}

export function useTags() {
  return useQuery({ queryKey: keys.tags, queryFn: api.tags });
}

/** Refreshes everything derived from notes after a change. */
export function useInvalidateNotes() {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: keys.notes });
    qc.invalidateQueries({ queryKey: keys.notebooks });
    qc.invalidateQueries({ queryKey: keys.tags });
    qc.invalidateQueries({ queryKey: keys.stats });
    qc.invalidateQueries({ queryKey: keys.public });
  };
}
