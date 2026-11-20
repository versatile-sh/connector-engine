import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Access, type ImportSource, type Preview } from "../api";

/** One import from preview to install: the server read once, the owner's changes to the proposal, and the install that sends them. */
export function useImport(source: ImportSource) {
  const queryClient = useQueryClient();
  const preview = useQuery({
    queryKey: ["preview", source],
    queryFn: () => api<Preview>("POST", "/api/preview", source),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const [choices, setChoices] = useState<Record<string, Access>>({});
  const install = useMutation({
    mutationFn: () => api<{ connector: string }>("POST", "/api/import", { ...source, choices }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["connectors"] }),
  });
  return {
    preview,
    install,
    accessOf: (name: string, proposed: Access) => choices[name] ?? proposed,
    choose: (name: string, access: Access) => setChoices((previous) => ({ ...previous, [name]: access })),
  };
}
