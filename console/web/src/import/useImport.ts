import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Access, type ImportSource, type Preview } from "../api";

/** One import from preview to install: the server or skill read once, the owner's changes to the proposal, and the install that sends them bound to what was shown. */
export function useImport(source: ImportSource) {
  const queryClient = useQueryClient();
  const preview = useQuery({
    queryKey: ["preview", source],
    queryFn: () => api<Preview>("POST", "/api/preview", source),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const [choices, setChoices] = useState<Record<string, Access>>({});
  const [skillAccess, setSkillAccess] = useState<Access>("on");
  const install = useMutation({
    mutationFn: () =>
      api<{ connector?: string; skill?: string }>(
        "POST",
        "/api/import",
        preview.data?.kind === "skill" ? { ...source, access: skillAccess, digest: preview.data.skill?.digest } : { ...source, choices },
      ),
    onSuccess: () =>
      Promise.all(["connectors", "skills", "session"].map((key) => queryClient.invalidateQueries({ queryKey: [key] }))),
  });
  return {
    preview,
    install,
    skillAccess,
    setSkillAccess,
    accessOf: (name: string, proposed: Access) => choices[name] ?? proposed,
    choose: (name: string, access: Access) => setChoices((previous) => ({ ...previous, [name]: access })),
  };
}
