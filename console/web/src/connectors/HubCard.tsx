import { hubURL } from "../hub";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { ACCENT_BAR, LIT_EDGE } from "../ui/styles";

/** The way to the connector hub: a card with a lime bar inset on its left, a top edge that lights on hover and a Hub pill that opens the catalogue, in the console's language, in a new tab. */
export function HubCard({ className = "" }: { className?: string }) {
  const t = useT();
  const href = hubURL(useLocale());
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener"
      className={`group bubble ${LIT_EDGE} flex items-center gap-4 border border-line-strong bg-paper/4 py-3.5 pr-4 pl-5 transition-[border-color,translate] duration-300 ease-(--ease-settle) hover:-translate-y-0.5 focus-visible:-translate-y-0.5 ${className}`}
    >
      <span aria-hidden="true" className={`${ACCENT_BAR} bg-lime`} />
      <span className="bubble-word shrink-0 bg-lime px-2 py-0.5 font-mono text-xs font-semibold text-ink">{t("hub.pill")}</span>
      <span className="min-w-0 flex-1">
        <span className="block text-[0.9375rem] leading-snug font-semibold [overflow-wrap:anywhere]">{t("hub.title", { where: href.replace(/^https?:\/\//, "") })}</span>
        <span className="block text-sm text-muted">{t("hub.body")}</span>
      </span>
      <Icon name="external" className="size-4 text-muted transition-colors group-hover:text-lime" />
      <span className="sr-only">{t("common.opensNewTab")}</span>
    </a>
  );
}
