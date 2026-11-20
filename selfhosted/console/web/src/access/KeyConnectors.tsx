import { useQuery } from "@tanstack/react-query";
import { profileHref } from "../connectors/profiles";
import { useT } from "../i18n/I18nProvider";
import { connectorsQuery } from "../queries";
import { navigate } from "../router";
import { Disclosure } from "../ui/Disclosure";
import { CAPTION, QUIET_LINK } from "../ui/styles";
import { Tag } from "../ui/Tag";

/** For one access key, every connector with the key's mode there (off, the default, or custom with how many rows it narrows) and a link to its profile on the connector's page, which stays the one place that edits it. */
export function KeyConnectors({ agent }: { agent: string }) {
  const t = useT();
  const connectors = useQuery(connectorsQuery);
  const rows = (connectors.data ?? []).flatMap((connector) => {
    const profile = connector.profiles.find((p) => p.agent === agent);
    return profile ? [{ connector, profile }] : [];
  });
  if (rows.length === 0) return null;
  const count = (mode: "off" | "custom") => rows.filter((row) => row.profile.mode === mode).length;
  return (
    <Disclosure
      className="-mx-4 mt-1 sm:-mx-5"
      summary={
        <>
          <span className="text-sm text-muted">{t("keys.connectors")}</span>
          <span className={CAPTION}>{t("keys.connectorsCounts", { count: rows.length, custom: count("custom"), off: count("off") })}</span>
        </>
      }
    >
      <ul className="px-4 pb-2 sm:px-5">
        {rows.map(({ connector, profile }) => (
          <li key={connector.id} className="flex items-center justify-between gap-3 py-1.5">
            <a
              href={profileHref(connector.id, agent)}
              onClick={(event) => {
                event.preventDefault();
                navigate(profileHref(connector.id, agent));
              }}
              className={`${QUIET_LINK} min-w-0 truncate text-sm`}
            >
              {connector.title}
            </a>
            <Tag tone={profile.mode === "custom" ? "paper" : "muted"} dot={profile.mode === "off"}>
              {profile.mode === "custom" ? t("keys.modeCustom", { count: Object.keys(profile.overrides).length }) : t(`profile.mode.${profile.mode}`)}
            </Tag>
          </li>
        ))}
      </ul>
    </Disclosure>
  );
}
