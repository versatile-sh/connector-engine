import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, errorText, type NotificationsData } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Notice } from "../ui/Notice";
import { Switch } from "../ui/Switch";
import { Section } from "./Section";

/** Ask in the AI app: MCP elicitation turned on per app, off by default, with the plain warning about apps that answer by themselves. */
export function ElicitSection({ apps }: { apps: NotificationsData["apps"] }) {
  const t = useT();
  const queryClient = useQueryClient();
  const toggle = useMutation({
    mutationFn: ({ name, elicit }: { name: string; elicit: boolean }) => api("PUT", `/api/notifications/apps/${encodeURIComponent(name)}`, { elicit }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["notifications"] }),
  });
  return (
    <Section id="elicit-title" icon="chat" title={t("notify.elicit.title")} intro={t("notify.elicit.intro")}>
      <p className="bubble border border-amber/40 border-l-4 border-l-amber bg-amber/5 py-2.5 pr-3 pl-4 text-sm">{t("notify.elicit.warning")}</p>
      {apps.length === 0 ? (
        <p className="mt-4 text-sm text-muted">{t("notify.elicit.empty")}</p>
      ) : (
        <ul className="mt-2">
          {apps.map((app) => (
            <li key={app.name} className="border-b border-line py-3 last:border-b-0">
              <Switch
                checked={toggle.isPending && toggle.variables.name === app.name ? toggle.variables.elicit : app.elicit}
                onChange={(elicit) => toggle.mutate({ name: app.name, elicit })}
                hint={t(app.elicit ? "notify.elicit.on" : "notify.elicit.off")}
              >
                <span className="font-mono" translate="no">
                  {app.name}
                </span>
              </Switch>
            </li>
          ))}
        </ul>
      )}
      {toggle.isError ? (
        <div className="mt-3">
          <Notice>{errorText(toggle.error)}</Notice>
        </div>
      ) : null}
    </Section>
  );
}
