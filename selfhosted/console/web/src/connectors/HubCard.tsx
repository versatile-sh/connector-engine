import { hubURL } from "../hub";
import { useLocale, useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { LIT_EDGE } from "../ui/styles";

/** The way to the connector hub as a card of the site's directory: what it holds and how Import works, its top edge lighting on hover, opening the catalogue in the console's language in a new tab. */
export function HubCard({ className = "" }: { className?: string }) {
  const t = useT();
  const href = hubURL(useLocale());
  return (
    <a href={href} target="_blank" rel="noopener" className={`group ${LIT_EDGE} flex items-center gap-4 px-5 py-4 ${className}`}>
      <span className="min-w-0 flex-1">
        <span className="block leading-snug font-medium">{t("hub.title", { where: href.replace(/^https?:\/\//, "") })}</span>
        <span className="mt-0.5 block text-sm text-muted">{t("hub.body")}</span>
      </span>
      <Icon name="external" className="size-4 text-muted transition-colors duration-250 group-hover:text-lime" />
      <span className="sr-only">{t("common.opensNewTab")}</span>
    </a>
  );
}
