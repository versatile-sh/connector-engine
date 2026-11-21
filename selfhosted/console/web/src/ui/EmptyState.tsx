import type { ReactNode } from "react";
import { Display } from "./Display";
import { Icon, type IconName } from "./Icon";
import { Marked } from "./Marked";
import { SURFACE, TITLE } from "./styles";

type EmptyStateProps = { title: string; body?: ReactNode; heading?: "h1" | "h2" | "h3"; icon?: IconName; bare?: boolean; children?: ReactNode };

/** What a view says while it has nothing to show: with `heading="h1"`, on a page of its own with no header (a link that no longer works), the serif headline with its lime word; anywhere else a compact centred card of an icon, one sans line, one muted line on what fills it and the way to fill it, never a second serif headline under the page's own. `bare` draws it with no card, for a place that is a card already. */
export function EmptyState({ title, body, heading = "h2", icon, bare = false, children }: EmptyStateProps) {
  if (heading === "h1") {
    return (
      <div className="arrive py-6 sm:py-10">
        <Display as="h1" size="title">
          <Marked text={title} />
        </Display>
        {body ? <p className="mt-4 max-w-xl text-pretty text-muted">{body}</p> : null}
        {children ? <div className="mt-8">{children}</div> : null}
      </div>
    );
  }
  const Heading = heading;
  return (
    <div className={`arrive mx-auto flex w-full max-w-xl flex-col items-center text-center ${bare ? "py-6" : `${SURFACE} px-6 py-8 sm:px-8`}`}>
      {icon ? (
        <span aria-hidden="true" className="mb-4 grid size-11 place-items-center rounded-full border border-line-strong text-muted">
          <Icon name={icon} className="size-5" />
        </span>
      ) : null}
      <Heading className={TITLE}>
        <Marked text={title} />
      </Heading>
      {body ? <p className="mt-2 max-w-md text-sm text-pretty text-muted">{body}</p> : null}
      {children ? <div className="mt-5 flex w-full flex-col items-center">{children}</div> : null}
    </div>
  );
}
