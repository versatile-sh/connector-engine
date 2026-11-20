import type { ReactNode } from "react";
import { Icon, type IconName } from "../ui/Icon";
import { SURFACE } from "../ui/styles";

type SectionProps = { id: string; icon: IconName; title: string; intro: string; action?: ReactNode; children: ReactNode };

/** One surface of the notifications page: an icon, a title, what it does in a line, an optional action beside the title, and its body. */
export function Section({ id, icon, title, intro, action, children }: SectionProps) {
  return (
    <section aria-labelledby={id} className={`${SURFACE} p-5 sm:p-6`}>
      <div className="flex flex-wrap items-start gap-3">
        <span aria-hidden="true" className="grid size-9 shrink-0 place-items-center rounded-full border border-line-strong text-muted">
          <Icon name={icon} className="size-[1.1rem]" />
        </span>
        <div className="min-w-0 flex-1">
          <h2 id={id} className="text-lg leading-snug font-medium">
            {title}
          </h2>
          <p className="mt-0.5 max-w-2xl text-sm text-muted">{intro}</p>
        </div>
        {action}
      </div>
      <div className="mt-5">{children}</div>
    </section>
  );
}
