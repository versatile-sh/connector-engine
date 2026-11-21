import type { ReactNode } from "react";
import { Display } from "./Display";
import { Marked } from "./Marked";

type EmptyStateProps = { title: string; body?: ReactNode; heading?: "h1" | "h2" | "h3"; children?: ReactNode };

/** What a page says while it has nothing to show, as the site says it: a serif headline with its lime word, a muted line on what fills it, and the way to fill it. */
export function EmptyState({ title, body, heading = "h2", children }: EmptyStateProps) {
  return (
    <div className="arrive py-6 sm:py-10">
      <Display as={heading} size="title">
        <Marked text={title} />
      </Display>
      {body ? <p className="mt-4 max-w-xl text-pretty text-muted">{body}</p> : null}
      {children ? <div className="mt-8">{children}</div> : null}
    </div>
  );
}
