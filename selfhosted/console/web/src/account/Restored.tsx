import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api";
import { Frame } from "../Frame";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Display } from "../ui/Display";
import { CAPTION, SURFACE } from "../ui/styles";
import { restarted } from "./backup";

/** The screen after a full backup is imported: it asks for `mise run unlock` and polls the console, through the errors of its restart, until it is ready; then every answer is read again and the console moves on to sign-in. */
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
    <Frame width="narrow">
      <section aria-labelledby="restored-title" className={`${SURFACE} arrive p-6 sm:p-8`}>
        <Display id="restored-title" size="title">
          {t("restored.title")}
        </Display>
        <p className="mt-4 text-pretty text-muted">{t("restored.body")}</p>
        <p role="status" className={`${CAPTION} mt-6 flex items-center gap-2`}>
          <span aria-hidden="true" className="pulse size-1.5 shrink-0 rounded-full bg-lime" />
          {t("restored.waiting")}
        </p>
      </section>
    </Frame>
  );
}
