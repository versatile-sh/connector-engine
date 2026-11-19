import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import type { MessageKey } from "../i18n";
import { connectorsQuery, keysQuery } from "../queries";

const KEY = "console.setup.hidden";
const listeners = new Set<() => void>();
let hiddenThisVisit = false;

const read = () => {
  try {
    return hiddenThisVisit || window.localStorage.getItem(KEY) === "1";
  } catch {
    return hiddenThisVisit;
  }
};

const persist = () => {
  try {
    window.localStorage.setItem(KEY, "1");
    return true;
  } catch {
    return false;
  }
};

const subscribe = (onChange: () => void) => {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
};

/** One step of the first-run guide: what it says and whether real data shows it done. */
export type SetupStep = { id: "account" | "connector" | "app"; title: MessageKey; done: boolean };

/** The first-run guide: its three steps ticked from the real connectors and keys, and whether the owner hid it. */
export function useSetup() {
  const connectors = useQuery(connectorsQuery);
  const keys = useQuery(keysQuery);
  const hidden = useSyncExternalStore(subscribe, read);
  const steps: SetupStep[] = [
    { id: "account", title: "onboarding.step1", done: true },
    { id: "connector", title: "onboarding.step2", done: (connectors.data?.length ?? 0) > 0 },
    { id: "app", title: "onboarding.step3", done: (keys.data?.agents.length ?? 0) > 0 },
  ];
  const done = steps.filter((step) => step.done).length;
  return {
    steps,
    done,
    total: steps.length,
    ready: connectors.isSuccess && keys.isSuccess,
    hidden,
    hide() {
      hiddenThisVisit = true;
      persist();
      listeners.forEach((listener) => listener());
    },
  };
}
