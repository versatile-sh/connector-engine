import type { Connector } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { navigate, openLink } from "../router";
import { Arrow } from "../ui/Icon";
import { CAPTION, QUIET_LINK, SKELETON_LINE, SURFACE, TITLE, button } from "../ui/styles";
import { askedOf } from "./recent";

const SHOWN = 5;

const NAMES = 3;

type AskFunctionsProps = { connectors: readonly Connector[] | undefined; failed: boolean };

/** The connectors and functions that stop for an answer: each connector with how many of its functions are at Ask and the first few by name, linking to its page, the first few connectors and a link to the rest; with none at Ask, how to set one, and a muted line when the connectors could not be read. */
export function AskFunctions({ connectors, failed }: AskFunctionsProps) {
  const t = useT();
  const asked = connectors ? askedOf(connectors) : undefined;
  return (
    <section aria-labelledby="ask-title" className={`${SURFACE} min-w-0 p-5`}>
      <h2 id="ask-title" className={TITLE}>
        {t("approvals.ask.title")}
      </h2>
      <p className="mt-1 text-sm text-pretty text-muted">{t("approvals.ask.intro")}</p>
      {!asked && failed ? (
        <p className="mt-4 text-sm text-pretty text-muted">{t("approvals.unavailable")}</p>
      ) : !asked ? (
        <div aria-hidden="true" className="mt-4 flex flex-col gap-3">
          {[0, 1].map((row) => (
            <div key={row} className={`${SKELETON_LINE} h-9 w-full`} />
          ))}
        </div>
      ) : asked.length === 0 ? (
        <div className="mt-4 flex flex-col items-start gap-3">
          <p className="text-sm text-pretty text-muted">{t("approvals.ask.empty")}</p>
          <button type="button" className={button({ variant: "secondary", size: "xs" })} onClick={() => navigate("/connectors")}>
            {t("approvals.ask.open")}
          </button>
        </div>
      ) : (
        <>
          <ul className="-mx-2 mt-3 flex flex-col divide-y divide-line">
            {asked.slice(0, SHOWN).map((row) => {
              const href = `/connectors/${encodeURIComponent(row.id)}`;
              return (
                <li key={row.id}>
                  <a href={href} onClick={openLink(href)} className="group grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 rounded-lg px-2 py-2.5 transition-colors duration-200 hover:bg-paper/5 focus-visible:bg-paper/5">
                    <ConnectorIcon src={row.icon} title={row.title} size="xs" />
                    <span className="min-w-0">
                      <span className="block truncate text-sm text-paper">{row.title}</span>
                      <span className={`${CAPTION} block truncate`} translate="no">
                        {row.names.slice(0, NAMES).join(", ")}
                        {row.names.length > NAMES ? ` +${row.names.length - NAMES}` : ""}
                      </span>
                    </span>
                    <span className="text-muted transition-[color,translate] duration-250 group-hover:translate-x-0.5 group-hover:text-paper">
                      <Arrow />
                    </span>
                  </a>
                </li>
              );
            })}
          </ul>
          {asked.length > SHOWN ? (
            <a href="/connectors" onClick={openLink("/connectors")} className={`${QUIET_LINK} mt-3 inline-block text-sm text-muted`}>
              {t("approvals.ask.more", { count: asked.length - SHOWN })}
            </a>
          ) : null}
        </>
      )}
    </section>
  );
}
