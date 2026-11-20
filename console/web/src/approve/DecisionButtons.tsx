import type { ApprovalChoice } from "../api";
import type { MessageKey } from "../i18n";
import { useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { button } from "../ui/styles";
import { offeredChoices } from "./choices";

type DecisionButtonsProps = { choices: readonly string[] | undefined; deciding?: ApprovalChoice | undefined; size?: "sm" | "md"; stacked?: boolean; onChoose(choice: ApprovalChoice): void };

const LABEL: Record<ApprovalChoice, MessageKey> = { once: "decide.once", session: "decide.session", always: "decide.always", deny: "decide.deny" };

const VARIANT = { once: "accent", session: "secondary", always: "secondary", deny: "danger" } as const;

/** The answers to one held call: Allow once, Allow for this session when the record offers it, Always allow and Deny. */
export function DecisionButtons({ choices, deciding, size = "sm", stacked = false, onChoose }: DecisionButtonsProps) {
  const t = useT();
  return (
    <div role="group" aria-label={t("decide.label")} aria-busy={deciding ? true : undefined} className={stacked ? "grid gap-2 sm:grid-cols-2" : "flex flex-wrap gap-2"}>
      {offeredChoices(choices).map((choice) => (
        <button
          key={choice}
          type="button"
          disabled={deciding !== undefined}
          className={`${button({ variant: VARIANT[choice], size })} ${stacked ? "w-full" : ""} ${deciding === choice ? "pulse" : ""}`}
          onClick={() => onChoose(choice)}
        >
          {choice === "once" ? <Icon name="check" className="size-4" /> : null}
          {t(LABEL[choice])}
        </button>
      ))}
    </div>
  );
}
