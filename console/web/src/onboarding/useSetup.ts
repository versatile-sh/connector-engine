import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Session } from "../api";
import type { MessageKey } from "../i18n";
import { connectorsQuery, keysQuery, sessionQuery } from "../queries";
import { guideVisible } from "./visibility";

/** One step of the first-run guide: what it says and whether real data shows it done. */
export type SetupStep = { id: "account" | "connector" | "app"; title: MessageKey; done: boolean };

/** The first-run guide: its three steps ticked from the real connectors and keys, whether it shows, and the dismissal the console's state keeps for good. */
export function useSetup() {
  const queryClient = useQueryClient();
  const session = useQuery(sessionQuery);
  const connectors = useQuery(connectorsQuery);
  const keys = useQuery(keysQuery);
  const dismiss = useMutation({
    mutationFn: () => api("POST", "/api/onboarding/dismiss"),
    onMutate: () => queryClient.setQueryData<Session>(sessionQuery.queryKey, (s) => s && { ...s, onboarding_dismissed: true }),
    onError: () => queryClient.invalidateQueries({ queryKey: sessionQuery.queryKey }),
  });
  const steps: SetupStep[] = [
    { id: "account", title: "onboarding.step1", done: true },
    { id: "connector", title: "onboarding.step2", done: (connectors.data?.length ?? 0) > 0 },
    { id: "app", title: "onboarding.step3", done: (keys.data?.agents.length ?? 0) > 0 },
  ];
  const done = steps.filter((step) => step.done).length;
  const ready = session.isSuccess && connectors.isSuccess && keys.isSuccess;
  return {
    steps,
    done,
    total: steps.length,
    settled: !session.isPending && !connectors.isPending && !keys.isPending,
    visible: guideVisible({ ready, dismissed: session.data?.onboarding_dismissed === true, done, total: steps.length }),
    hide: () => dismiss.mutate(),
  };
}
