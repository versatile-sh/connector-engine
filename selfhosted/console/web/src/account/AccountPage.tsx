import { useQuery } from "@tanstack/react-query";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { accountQuery } from "../queries";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { SKELETON } from "../ui/styles";
import { SignInSection } from "./SignInSection";

/** The owner's account: how they sign in. */
export function AccountPage() {
  const t = useT();
  const account = useQuery(accountQuery);
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8">
        <PageTitle section={t("nav.account")} title={t("account.title")} />
        <p className="mt-3 max-w-2xl text-muted">{t("account.intro")}</p>
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
        </div>
      ) : null}
    </section>
  );
}
