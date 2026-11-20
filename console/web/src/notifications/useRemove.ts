import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useReducer } from "react";
import { api, errorText } from "../api";
import { revokeStep, type RevokeEvent } from "../access/revoke";

/** The inline remove flow of one list on the notifications page: one row asked at a time, DELETE on confirm, the list refetched after. */
export function useRemove(path: (id: string) => string) {
  const queryClient = useQueryClient();
  const [flow, dispatch] = useReducer(revokeStep, { kind: "idle" });
  const remove = useMutation({
    mutationFn: (id: string) => api("DELETE", path(id)),
    onSuccess: async () => {
      dispatch({ type: "revoked" });
      await queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
    onError: () => dispatch({ type: "failed" }),
  });
  const send = (event: RevokeEvent) => {
    if (event.type === "ask") remove.reset();
    dispatch(event);
  };
  return {
    returning: (id: string) => flow.kind === "idle" && flow.returnTo === id,
    asking: (id: string) => (flow.kind === "confirming" || flow.kind === "revoking") && flow.name === id,
    busy: flow.kind === "revoking",
    error: remove.isError ? errorText(remove.error) : undefined,
    ask: (id: string) => send({ type: "ask", name: id }),
    cancel: () => send({ type: "cancel" }),
    confirm: () => {
      if (flow.kind !== "confirming") return;
      dispatch({ type: "confirm" });
      remove.mutate(flow.name);
    },
  };
}
