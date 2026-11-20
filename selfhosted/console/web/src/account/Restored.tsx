import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api";
import { Mark } from "../Brand";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { restarted } from "./backup";

/** The screen after a full backup is imported: it asks for `make unlock` and polls the console, through the errors of its restart, until it is ready; then every answer is read again and the console moves on to sign-in. */
export function Restored() {
  const t = useT();
  const client = useQueryClient();
  const [startedAt] = useState(Date.now);
  const wentDown = useRef(false);
  useQuery({
    queryKey: ["restart", startedAt],
    queryFn: async () => {
      const ready = await api<{ ready: boolean }>("GET", "/api/ready").then(
        (answer) => answer.ready,
        () => false,
      );
      if (!ready) wentDown.current = true;
      if (restarted(ready, wentDown.current, Date.now() - startedAt)) {
        await client.resetQueries({ predicate: (query) => query.queryKey[0] !== "restart" });
        navigate("/", { replace: true });
      }
      return ready;
    },
    refetchInterval: 2_000,
    refetchIntervalInBackground: true,
  });
  return (
    <main className="grid min-h-dvh place-items-center px-6">
      <div className="flex max-w-md flex-col items-center gap-4 text-center">
        <Mark className="pulse size-12" />
        <h1 className="text-lit font-serif text-5xl leading-none font-normal tracking-[-0.02em]">{t("restored.title")}</h1>
        <p>{t("restored.body")}</p>
        <p role="status" className="text-sm text-muted">
          {t("restored.waiting")}
        </p>
      </div>
    </main>
  );
}
