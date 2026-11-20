import { useQuery } from "@tanstack/react-query";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { accountQuery } from "../queries";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { SKELETON } from "../ui/styles";
import { BackupSection } from "./BackupSection";
import { SignInSection } from "./SignInSection";

/** The owner's account: how they sign in, and the backups that move, restore or share this install. */
export function AccountPage() {
  const t = useT();
  const account = useQuery(accountQuery);
  return (
    <section aria-labelledby="page-title">
      <header className="mb-10">
        <PageTitle section={t("nav.account")} title={t("account.title")}>
          <p>{t("account.intro")}</p>
        </PageTitle>
      </header>
      {account.isError ? <Notice>{errorText(account.error)}</Notice> : null}
      {account.isPending ? (
        <div aria-hidden="true" className="flex flex-col gap-6">
          {[0, 1].map((row) => (
            <div key={row} className={`${SKELETON} h-40`} />
          ))}
        </div>
      ) : account.data ? (
        <div className="flex flex-col gap-6">
          <SignInSection username={account.data.username} />
          <BackupSection lastExport={account.data.last_export} />
        </div>
      ) : null}
    </section>
  );
}
