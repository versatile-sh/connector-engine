import type { ReactNode } from "react";
import { HubCard } from "../connectors/HubCard";
import { PasteBox } from "../connectors/PasteBox";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Arrow, Icon } from "../ui/Icon";
import { HAIRLINE, ICON_BUTTON, button } from "../ui/styles";
import { useSetup, type SetupStep } from "./useSetup";

function Reveal({ open, children }: { open: boolean; children: ReactNode }) {
  return (
    <div inert={!open} className={`grid transition-[grid-template-rows,opacity] duration-500 ease-(--ease-glide) ${open ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"}`}>
      <div className="overflow-hidden">{children}</div>
    </div>
  );
}

function Tick({ step, current }: { step: SetupStep; current: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`grid size-7 shrink-0 place-items-center rounded-full border transition-colors duration-500 ${step.done ? "border-transparent bg-lime text-ink" : current ? "border-lime text-lime" : "border-line-strong text-muted-light"}`}
    >
      {step.done ? <Icon name="check" className="size-4" /> : current ? <span className="pulse size-2 rounded-full bg-lime" /> : null}
    </span>
  );
}

/** The first-run guide on the connectors page: account, first connector, first AI app, each ticked from real data, the next one open with what it takes. It hides by itself once every step is done, and for good once the owner closes it. */
export function SetupGuide({ onPaste }: { onPaste(text: string): void }) {
  const t = useT();
  const setup = useSetup();
  if (!setup.visible) return null;
  const current = setup.steps.find((step) => !step.done)?.id;
  return (
    <section aria-labelledby="setup-title" className="arrive vignette-top bubble relative mb-8 overflow-hidden border border-line-strong bg-ink-raised">
      <span aria-hidden="true" className={HAIRLINE} />
      <header className="flex items-center gap-4 px-5 pt-4 pb-3 sm:px-6">
        <div className="min-w-0 flex-1">
          <h2 id="setup-title" className="font-serif text-2xl leading-tight font-normal tracking-[-0.01em]">
            {t("onboarding.title")}
          </h2>
          <p className="font-mono text-xs text-muted">{t("onboarding.progress", { done: setup.done, total: setup.total })}</p>
        </div>
        <button type="button" aria-label={t("onboarding.hide")} title={t("onboarding.hide")} className={ICON_BUTTON} onClick={setup.hide}>
          <Icon name="close" className="size-4" />
        </button>
      </header>
      <div aria-hidden="true" className="mx-5 h-1 overflow-hidden rounded-full bg-line sm:mx-6">
        <div className="h-full rounded-full bg-lime transition-[width] duration-700 ease-(--ease-glide)" style={{ width: `${(setup.done / setup.total) * 100}%` }} />
      </div>
      <ol className="px-5 py-3 sm:px-6">
        {setup.steps.map((step) => {
          const open = step.id === current;
          return (
            <li key={step.id} className="border-b border-line py-3 last:border-b-0">
              <div className="flex items-center gap-3">
                <Tick step={step} current={open} />
                <span className={`font-semibold transition-colors duration-300 ${step.done ? "text-muted" : "text-paper"}`}>{t(step.title)}</span>
                {step.done ? <span className="sr-only">{t("onboarding.stepDone")}</span> : null}
              </div>
              {step.id === "connector" ? (
                <Reveal open={open}>
                  <div className="pt-3 sm:pl-10">
                    <p className="mb-3 text-sm text-muted">{t("onboarding.step2.body")}</p>
                    <HubCard />
                    <PasteBox onPaste={onPaste} className="mt-4" />
                  </div>
                </Reveal>
              ) : null}
              {step.id === "app" ? (
                <Reveal open={open}>
                  <div className="flex flex-col items-start gap-3 pt-3 sm:flex-row sm:items-center sm:justify-between sm:pl-10">
                    <p className="text-sm text-muted">{t("onboarding.step3.body")}</p>
                    <button type="button" className={button({ variant: "accent", size: "sm" })} onClick={() => navigate("/access")}>
                      {t("onboarding.step3.action")}
                      <Arrow />
                    </button>
                  </div>
                </Reveal>
              ) : null}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
