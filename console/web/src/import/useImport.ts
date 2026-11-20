import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Access, type ImportSource, type Preview } from "../api";
import { installImport, type ImportRequest } from "../mutations";

/** One import from preview to install: the server or skill read once, the owner's changes to the proposal, and the request the install sends them in, bound to what was shown. */
export function useImport(source: ImportSource) {
  const preview = useQuery({
    queryKey: ["preview", source],
    queryFn: () => api<Preview>("POST", "/api/preview", source),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const [choices, setChoices] = useState<Record<string, Access>>({});
  const [skillAccess, setSkillAccess] = useState<Access>("on");
  const install = useMutation(installImport);
  const request: ImportRequest = preview.data?.kind === "skill" ? { ...source, access: skillAccess, digest: preview.data.skill?.digest } : { ...source, choices };
  return {
    preview,
    install,
    request,
    skillAccess,
    setSkillAccess,
    accessOf: (name: string, proposed: Access) => choices[name] ?? proposed,
    choose: (name: string, access: Access) => setChoices((previous) => ({ ...previous, [name]: access })),
  };
}
