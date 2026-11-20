import { HubCard } from "../connectors/HubCard";
import { PasteBox } from "../connectors/PasteBox";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Collapse } from "../ui/Collapse";
import { Arrow, Icon } from "../ui/Icon";
import { CAPTION, HAIRLINE, ICON_BUTTON, SURFACE, TITLE, button } from "../ui/styles";
import { useSetup, type SetupStep } from "./useSetup";

function Tick({ step, index, current }: { step: SetupStep; index: number; current: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`grid size-7 shrink-0 place-items-center rounded-full border font-mono text-xs transition-colors duration-250 ${step.done ? "border-line-strong text-lime" : current ? "border-paper/60 text-paper" : "border-line text-muted-light"}`}
    >
      {step.done ? <Icon name="check" className="size-4" /> : index + 1}
    </span>
  );
}

/** The first-run guide on the connectors page, in the card that holds the next step: account, first connector, first AI app, each ticked from real data with the site's lime check, the next one open with what it takes. It hides by itself once every step is done, and for good once the owner closes it. */
export function SetupGuide({ onPaste }: { onPaste(text: string): void }) {
  const t = useT();
  const setup = useSetup();
  if (!setup.visible) return null;
  const current = setup.steps.find((step) => !step.done)?.id;
  return (
    <section aria-labelledby="setup-title" className={`${SURFACE} arrive relative mb-8 overflow-hidden`}>
      <span aria-hidden="true" className={HAIRLINE} />
      <header className="flex items-center gap-4 px-5 pt-5 pb-2 sm:px-6">
        <div className="min-w-0 flex-1">
          <h2 id="setup-title" className={TITLE}>
            {t("onboarding.title")}
          </h2>
          <p className={CAPTION}>{t("onboarding.progress", { done: setup.done, total: setup.total })}</p>
        </div>
        <button type="button" aria-label={t("onboarding.hide")} title={t("onboarding.hide")} className={ICON_BUTTON} onClick={setup.hide}>
          <Icon name="close" className="size-4" />
        </button>
      </header>
      <ol className="px-5 pb-3 sm:px-6">
        {setup.steps.map((step, index) => {
          const open = step.id === current;
          return (
            <li key={step.id} className="border-b border-line py-3.5 last:border-b-0">
              <div className="flex items-center gap-3">
                <Tick step={step} index={index} current={open} />
                <span className={`font-medium transition-colors duration-250 ${step.done ? "text-muted" : "text-paper"}`}>{t(step.title)}</span>
                {step.done ? <span className="sr-only">{t("onboarding.stepDone")}</span> : null}
              </div>
              {step.id === "connector" ? (
                <Collapse id={`setup-${step.id}`} open={open}>
                  <div className="pt-4 sm:pl-10">
                    <p className="mb-4 text-sm text-pretty text-muted">{t("onboarding.step2.body")}</p>
                    <HubCard />
                    <PasteBox onPaste={onPaste} className="mt-4" />
                  </div>
                </Collapse>
              ) : null}
              {step.id === "app" ? (
                <Collapse id={`setup-${step.id}`} open={open}>
                  <div className="flex flex-col items-start gap-3 pt-4 sm:flex-row sm:items-center sm:justify-between sm:pl-10">
                    <p className="text-sm text-pretty text-muted">{t("onboarding.step3.body")}</p>
                    <button type="button" className={button({ size: "sm" })} onClick={() => navigate("/access")}>
                      {t("onboarding.step3.action")}
                      <Arrow />
                    </button>
                  </div>
                </Collapse>
              ) : null}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
