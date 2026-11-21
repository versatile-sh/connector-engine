import type { Ref } from "react";
import { Icon } from "../ui/Icon";
import { useT } from "../i18n/I18nProvider";
import { useSetup } from "../onboarding/useSetup";
import { useViewer } from "../people/useViewer";
import { navigate } from "../router";
import { button, CAPTION, ICON_BUTTON, PRESS } from "../ui/styles";
import { useCopy } from "../ui/useCopy";
import { crumbOf, type Section } from "./shell";

type TopStripProps = { section: Section; endpoint: string; sidebar: string; drawer: boolean; menuRef: Ref<HTMLButtonElement>; onOpenMenu(): void };

function Endpoint({ url }: { url: string }) {
  const t = useT();
  const { result, copy } = useCopy();
  return (
    <button type="button" onClick={() => void copy(url)} aria-label={t("shell.copyEndpoint", { url })} title={url} className={`inline-flex h-9 max-w-[12rem] min-w-0 items-center gap-2 rounded-full border border-line-strong px-3.5 font-mono text-xs text-muted hover:border-paper/50 hover:text-paper sm:max-w-xs ${PRESS}`}>
      <span className="size-1.5 shrink-0 rounded-full bg-lime" aria-hidden="true" />
      <span className="min-w-0 truncate" translate="no">{url.replace(/^https?:\/\//, "")}</span>
      <Icon name={result === "copied" ? "check" : "copy"} className={`size-3.5 ${result === "copied" ? "text-lime" : ""}`} />
    </button>
  );
}

function SetupProgress() {
  const t = useT();
  const setup = useSetup();
  const manage = useViewer().can("connectors.manage");
  if (!setup.visible || !manage) return null;
  return (
    <button type="button" onClick={() => navigate("/")} className={`${button({ variant: "secondary", size: "sm" })} arrive hidden font-mono text-xs font-normal sm:inline-flex`}>
      <span aria-hidden="true" className="flex gap-1">
        {setup.steps.map((step) => (
          <span key={step.id} className={`size-1.5 rounded-full transition-colors duration-250 ${step.done ? "bg-lime" : "bg-line-strong"}`} />
        ))}
      </span>
      {t("shell.setup", { done: setup.done, total: setup.total })}
    </button>
  );
}

/** The strip over the page, beside the sidebar: on a narrow screen the button that opens the drawer, then the section's place in the sidebar, the setup progress and the endpoint to copy. */
export function TopStrip({ section, endpoint, sidebar, drawer, menuRef, onOpenMenu }: TopStripProps) {
  const t = useT();
  const crumb = crumbOf(section);
  return (
    <header className="flex h-16 shrink-0 items-center gap-3 px-4 sm:px-6 lg:pr-3 lg:pl-6">
      <button ref={menuRef} type="button" aria-label={t("shell.openMenu")} aria-controls={sidebar} aria-expanded={drawer} className={`${ICON_BUTTON} -ml-2 lg:hidden`} onClick={onOpenMenu}>
        <Icon name="menu" className="size-[1.15rem]" />
      </button>
      <p className={`${CAPTION} min-w-0 flex-1 truncate`}>
        <span className="max-sm:hidden">
          {t(crumb.group)}
          <span aria-hidden="true" className="px-1.5 text-muted-light">/</span>
        </span>
        <span className="text-paper">{t(crumb.label)}</span>
      </p>
      <SetupProgress />
      <Endpoint url={endpoint} />
    </header>
  );
}
