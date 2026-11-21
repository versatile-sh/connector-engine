import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { SetupGuide } from "../onboarding/SetupGuide";
import { useSetup } from "../onboarding/useSetup";
import { useViewer } from "../people/useViewer";
import { connectorsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { EmptyState } from "../ui/EmptyState";
import { LongList } from "../ui/LongList";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageHeader } from "../ui/PageHeader";
import { CARD_GRID, PAGE, button } from "../ui/styles";
import { ConnectorCard, ConnectorCardSkeleton } from "./ConnectorCard";
import { HubCard } from "./HubCard";
import { PasteBox } from "./PasteBox";

/** Every installed connector as a grid of cards straight on the content surface, with a filter in the toolbar past six of them, the first-run guide above it, and the import window. `/import` with no link opens the window at once. */
export function ConnectorsPage() {
  const t = useT();
  const { path } = useLocation();
  const connectors = useQuery(connectorsQuery);
  const setup = useSetup();
  const manage = useViewer().can("connectors.manage");
  const [importing, setImporting] = useState<{ initial?: string } | undefined>(path === "/import" ? {} : undefined);
  const open = (initial?: string) => setImporting(initial ? { initial } : {});
  const close = () => {
    setImporting(undefined);
    if (path === "/import") navigate("/", { replace: true });
  };
  const list = connectors.data ?? [];
  const empty = connectors.isSuccess && list.length === 0;
  const covered = empty && manage && !(setup.settled && !setup.visible);
  return (
    <section aria-labelledby="page-title" className={PAGE}>
      <PageHeader
        title={t("connectors.title")}
        count={connectors.isSuccess ? list.length : undefined}
        actions={
          manage ? (
            <button type="button" className={button()} onClick={() => open()}>
              <Icon name="plus" className="size-4" />
              {t("connectors.import")}
            </button>
          ) : null
        }
      >
        {t("connectors.intro")}
      </PageHeader>
      {manage ? <SetupGuide onPaste={open} /> : null}
      {covered ? null : connectors.isError ? (
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
        <ul className={CARD_GRID}>
          {Array.from({ length: 8 }, (_, card) => (
            <ConnectorCardSkeleton key={card} />
          ))}
        </ul>
      ) : empty ? (
        <EmptyState icon="plug" title={t("connectors.empty.title")} body={manage ? t("connectors.empty.body") : t("connectors.empty.byAdmins")}>
          {manage ? (
            <div className="flex w-full flex-col gap-5 text-left">
              <HubCard />
              <PasteBox onPaste={open} />
            </div>
          ) : null}
        </EmptyState>
      ) : (
        <LongList rows={list} textOf={(c) => `${c.title} ${c.alias} ${c.publisher ?? ""} ${c.description ?? ""}`} keyOf={(c) => c.id} label={t("connectors.filter")} whole bar>
          {(shown) => (
            <ul className={CARD_GRID}>
              {shown.map(({ row: connector, phase }) => (
                <ConnectorCard key={connector.id} phase={phase} connector={connector} />
              ))}
            </ul>
          )}
        </LongList>
      )}
      {importing && manage ? <ImportDialog initial={importing.initial} onClose={close} /> : null}
    </section>
  );
}
