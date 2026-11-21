import type { ReactNode } from "react";
import { Icon } from "./Icon";

type NoticeProps = { tone?: "amber" | "lime"; live?: boolean; action?: ReactNode; className?: string; children: ReactNode };

const TONE = { amber: "border-amber-400/40 bg-amber-400/5", lime: "border-line-strong bg-(color:--card-fill)" } as const;

/** The one note box: amber when something went wrong or needs care, a calm box with the lime success check when it went well; `live` announces it, else it is read in place. */
export function Notice({ tone = "amber", live = true, action, className = "", children }: NoticeProps) {
  return (
    <div role={live ? (tone === "amber" ? "alert" : "status") : undefined} className={`arrive flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border px-4 py-3 text-sm ${TONE[tone]} ${className}`}>
      {tone === "lime" ? <Icon name="check" className="size-4 text-lime" /> : null}
      <div className="min-w-0 flex-1 basis-56">{children}</div>
      {action}
    </div>
  );
}
