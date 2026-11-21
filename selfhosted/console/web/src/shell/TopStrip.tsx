import type { Ref } from "react";
import { Icon } from "../ui/Icon";
import { useT } from "../i18n/I18nProvider";
import { useSetup } from "../onboarding/useSetup";
import { useViewer } from "../people/useViewer";
import { navigate } from "../router";
import { button, CAPTION, ICON_BUTTON } from "../ui/styles";
import { crumbOf, type Section } from "./shell";

type TopStripProps = { section: Section; sidebar: string; drawer: boolean; menuRef: Ref<HTMLButtonElement>; onOpenMenu(): void };

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

/** The strip over the page, beside the sidebar: on a narrow screen the button that opens the drawer, then the section's place in the sidebar and the setup progress. */
export function TopStrip({ section, sidebar, drawer, menuRef, onOpenMenu }: TopStripProps) {
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
    </header>
  );
}
