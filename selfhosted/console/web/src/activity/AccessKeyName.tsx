import { useQuery } from "@tanstack/react-query";
import { useT } from "../i18n/I18nProvider";
import { keysQuery } from "../queries";
import { navigate } from "../router";
import { CAPTION, QUIET_LINK } from "../ui/styles";

/** The access key a call used; `detailed` adds how the key was made and links it to the Access page, so it stays plain text inside a clickable row. */
export function AccessKeyName({ name, detailed = false }: { name: string; detailed?: boolean }) {
  const t = useT();
  const keys = useQuery(keysQuery).data;
  if (!detailed) {
    return (
      <span className="truncate" translate="no">
        <span className="sr-only">{t("activity.key")}: </span>
        {name}
      </span>
    );
  }
  const key = keys?.agents.find((agent) => agent.name === name);
  const how = !keys ? null : !key ? t("activity.key.gone") : key.via_browser ? t("activity.key.browser") : t("activity.key.manual");
  return (
    <span className="inline-flex min-w-0 flex-wrap items-baseline gap-x-2">
      <a
        href="/access"
        onClick={(event) => {
          event.preventDefault();
          navigate("/access");
        }}
        className={`${QUIET_LINK} truncate font-mono`}
        translate="no"
      >
        {name}
      </a>
      {how ? <span className={CAPTION}>{how}</span> : null}
    </span>
  );
}
