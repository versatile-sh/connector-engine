import { useQuery } from "@tanstack/react-query";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { PeopleTab } from "../people/PeopleTab";
import { useViewer } from "../people/useViewer";
import { accountQuery } from "../queries";
import { navigate } from "../router";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { PAGE, SKELETON } from "../ui/styles";
import { Tabs, tabId } from "../ui/Tabs";
import { BackupSection } from "./BackupSection";
import { SignInSection } from "./SignInSection";
import { accountPathOf, accountTabOf, accountTabsOf, type AccountTab } from "./tabs";

const PANEL_ID = "account-panel";

function Own({ tab }: { tab: "profile" | "backups" }) {
  const account = useQuery(accountQuery);
  if (account.isError) return <Notice>{errorText(account.error)}</Notice>;
  if (!account.data) return <div aria-hidden="true" className={`${SKELETON} h-40`} />;
  return tab === "profile" ? <SignInSection username={account.data.username} /> : <BackupSection lastExport={account.data.last_export} />;
}

/** The signed-in user's account under underline tabs, each one a path: Profile (how they sign in), People and Backups for those the server lets open them; with Profile alone there is no tab bar. */
export function AccountPage({ path }: { path: string }) {
  const t = useT();
  const tabs = accountTabsOf(useViewer());
  const tab = accountTabOf(path, tabs);
  const many = tabs.length > 1;
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader title={t("account.title")}>
        <p>{t("account.intro")}</p>
      </PageHeader>
      {many ? <Tabs<AccountTab> label={t("account.tabs")} value={tab} options={tabs.map((value) => ({ value, text: t(`account.tab.${value}`) }))} onChange={(next) => navigate(accountPathOf(next), { keepScroll: true })} panel={PANEL_ID} /> : null}
      <div id={PANEL_ID} role={many ? "tabpanel" : undefined} aria-labelledby={many ? tabId(PANEL_ID, tab) : undefined} className="flex flex-col gap-6">
        {tab === "people" ? <PeopleTab /> : <Own tab={tab} />}
      </div>
    </section>
  );
}
