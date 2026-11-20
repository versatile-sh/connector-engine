import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { ImportDialog } from "../import/ImportDialog";
import { SetupGuide } from "../onboarding/SetupGuide";
import { useSetup } from "../onboarding/useSetup";
import { connectorsQuery } from "../queries";
import { navigate, useLocation } from "../router";
import { Bubble } from "../ui/Bubble";
import { Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { PageTitle } from "../ui/PageTitle";
import { button } from "../ui/styles";
import { ConnectorCard, ConnectorCardSkeleton } from "./ConnectorCard";
import { HubCard } from "./HubCard";
import { PasteBox } from "./PasteBox";

function Empty({ onPaste }: { onPaste(text: string): void }) {
  const t = useT();
  return (
    <section className="arrive bubble grid overflow-hidden border border-line-strong lg:grid-cols-2">
      <div className="relative isolate bg-lime p-6 text-ink sm:p-8">
        <div aria-hidden="true" className="backdrop-grid-ink absolute inset-0 -z-10" />
        <h2 className="font-serif text-4xl leading-[0.98] font-normal tracking-[-0.02em] sm:text-5xl">{t("connectors.empty.title")}</h2>
        <p className="mt-3 max-w-md text-ink/80">{t("connectors.empty.body")}</p>
      </div>
      <div className="flex flex-col justify-center gap-5 bg-ink-raised p-6 sm:p-8">
        <HubCard />
        <PasteBox onPaste={onPaste} />
      </div>
    </section>
  );
}

/** Every installed connector as a grid of cards, the first-run guide above them, and the import window. `/import` with no link opens the window at once. */
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
  return (
    <section aria-labelledby="page-title">
      <header className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <PageTitle section={t("nav.connectors")} title={t("connectors.title")}>
            {connectors.isSuccess ? <Bubble tone={list.length > 0 ? "accent" : "neutral"}>{list.length}</Bubble> : null}
          </PageTitle>
          <p className="mt-3 text-muted">{t("connectors.intro")}</p>
        </div>
        <button type="button" className={button({ variant: "accent" })} onClick={() => open()}>
          <Icon name="plus" className="size-4" />
          {t("connectors.import")}
        </button>
      </header>
      <SetupGuide onPaste={open} />
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
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }, (_, card) => (
            <ConnectorCardSkeleton key={card} />
          ))}
        </ul>
      ) : list.length === 0 ? (
        setup.settled && !setup.visible ? (
          <Empty onPaste={open} />
        ) : null
      ) : (
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {list.map((connector) => (
            <ConnectorCard key={connector.id} connector={connector} />
          ))}
        </ul>
      )}
      {importing ? <ImportDialog initial={importing.initial} onClose={close} /> : null}
    </section>
  );
}
