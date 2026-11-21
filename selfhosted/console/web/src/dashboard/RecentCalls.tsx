import type { ActivityCall } from "../api";
import { CallList } from "../activity/CallList";
import { hrefOf, type Named } from "../activity/activity";
import { useT } from "../i18n/I18nProvider";
import { navigate, openLink } from "../router";
import { Arrow } from "../ui/Icon";
import { QUIET_LINK, SURFACE, TITLE } from "../ui/styles";

type RecentCallsProps = { calls: readonly ActivityCall[]; named: ReadonlyMap<string, Named>; owners: boolean; scope: string };

/** The newest calls in a card as on the activity page, with a link to all of it; a row opens that call over the activity of the last seven days. */
export function RecentCalls({ calls, named, owners, scope }: RecentCallsProps) {
  const t = useT();
  const all = hrefOf({ range: "7d", ...(owners ? { scope: "everyone" as const } : {}) });
  return (
    <section aria-labelledby="recent-title" className={`${SURFACE} min-w-0 p-4 sm:p-5`}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 px-1">
        <h2 id="recent-title" className={TITLE}>
          {t("dashboard.recent")}
        </h2>
        <a href={all} onClick={openLink(all)} className={`${QUIET_LINK} inline-flex items-center gap-1.5 text-sm text-muted`}>
          {t("dashboard.recent.all")}
          <Arrow />
        </a>
      </div>
      <div className="mt-2">
        <CallList calls={calls} named={named} owners={owners} scope={`dashboard:${scope}`} onOpen={(id) => navigate(hrefOf({ range: "7d", ...(owners ? { scope: "everyone" as const } : {}) }, id))} />
      </div>
    </section>
  );
}
