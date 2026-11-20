import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { SetupGuide } from "../onboarding/SetupGuide";
import { useSetup } from "../onboarding/useSetup";
import { connectorsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { PANEL, button } from "../ui/styles";
import { ConnectorCard, ConnectorCardSkeleton } from "./ConnectorCard";
import { HubCard } from "./HubCard";
import { PasteBox } from "./PasteBox";

const GRID = "grid gap-3 @xl:grid-cols-2 @4xl:grid-cols-3";

/** Every installed connector as a grid of cards in one panel, the first-run guide above it, and the import window. `/import` with no link opens the window at once. */
export function ConnectorsPage() {
  const t = useT();
  const { path } = useLocation();
  const connectors = useQuery(connectorsQuery);
  const setup = useSetup();
  const [importing, setImporting] = useState<{ initial?: string } | undefined>(path === "/import" ? {} : undefined);
  const open = (initial?: string) => setImporting(initial ? { initial } : {});
  const close = () => {
    setImporting(undefined);
    if (path === "/import") navigate("/", { replace: true });
  };
  const list = connectors.data ?? [];
  const empty = connectors.isSuccess && list.length === 0;
  return (
    <section aria-labelledby="page-title">
      <header className="mb-10 flex flex-wrap items-end justify-between gap-6">
        <div className="min-w-0">
          <PageTitle section={t("nav.connectors")} title={t("connectors.title")} count={connectors.isSuccess ? list.length : undefined}>
            {t("connectors.intro")}
          </PageTitle>
        </div>
        <button type="button" className={button()} onClick={() => open()}>
          <Icon name="plus" className="size-4" />
          {t("connectors.import")}
        </button>
      </header>
      <SetupGuide onPaste={open} />
      {empty && !(setup.settled && !setup.visible) ? null : (
        <div className={`${PANEL} @container p-4 sm:p-6 lg:p-8`}>
          {connectors.isError ? (
            <Notice
              action={
                <button type="button" className={button({ variant: "secondary", size: "xs" })} onClick={() => void connectors.refetch()}>
                  {t("common.retry")}
                </button>
              }
            >
              {errorText(connectors.error)}
            </Notice>
          ) : connectors.isPending ? (
            <ul className={GRID}>
              {Array.from({ length: 6 }, (_, card) => (
                <ConnectorCardSkeleton key={card} />
              ))}
            </ul>
          ) : empty ? (
            <EmptyState title={t("connectors.empty.title")} body={t("connectors.empty.body")}>
              <div className="flex max-w-2xl flex-col gap-5">
                <HubCard />
                <PasteBox onPaste={open} />
              </div>
            </EmptyState>
          ) : (
            <ul className={GRID}>
              {list.map((connector) => (
                <ConnectorCard key={connector.id} connector={connector} />
              ))}
            </ul>
          )}
        </div>
      )}
      {importing ? <ImportDialog initial={importing.initial} onClose={close} /> : null}
    </section>
  );
}
