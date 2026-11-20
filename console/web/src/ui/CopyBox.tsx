import { useT } from "../i18n/I18nProvider";
import { Icon } from "./Icon";
import { button } from "./styles";
import { useCopy } from "./useCopy";

type CopyBoxProps = { label: string; text: string; tone?: "card" | "sunken"; className?: string };

/** A labelled block of text in mono that wraps inside its box, with one Copy button that keeps its width whatever it says. */
export function CopyBox({ label, text, tone = "sunken", className = "" }: CopyBoxProps) {
  const t = useT();
  const { result, copy } = useCopy();
  return (
    <div className={`bubble border border-line-strong ${tone === "sunken" ? "bg-ink-sunken" : "bg-ink-raised"} ${className}`}>
      <div className="flex items-center justify-between gap-3 border-b border-line py-1.5 pr-1.5 pl-4">
        <span className="min-w-0 font-mono text-xs text-muted [overflow-wrap:anywhere]">{label}</span>
        <button type="button" onClick={() => void copy(text)} className={`${button({ variant: result === "copied" ? "accent" : "secondary", size: "xs" })} w-30`}>
          <Icon name={result === "copied" ? "check" : "copy"} className="size-3.5" />
          <span aria-live="polite">{t(result === "copied" ? "common.copied" : result === "failed" ? "common.copyFailed" : "common.copy")}</span>
        </button>
      </div>
      <pre className="px-4 py-3 font-mono text-[0.8125rem] leading-relaxed whitespace-pre-wrap text-paper select-all [overflow-wrap:anywhere]">{text}</pre>
    </div>
  );
}
