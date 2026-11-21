import type { UseQueryResult } from "@tanstack/react-query";
import type { Preview } from "../api";
import { ConnectorIcon } from "../connectors/ConnectorIcon";
import { useT } from "../i18n/I18nProvider";
import { Display } from "../ui/Display";
import { Icon } from "../ui/Icon";
import { CAPTION, QUIET_LINK, SKELETON_LINE } from "../ui/styles";
import { MetaChips } from "./MetaChips";

const isWeb = (raw: string) => /^https?:\/\//i.test(raw);

function Provenance({ preview, host }: { preview: Preview; host: string }) {
  const t = useT();
  const text = t(preview.kind === "mcp" ? "import.fromServer" : "import.fromCatalog", { host });
  const href = preview.page_url ?? preview.url;
  return (
    <p className="mt-4 flex max-w-full justify-center text-sm text-muted lg:justify-start">
      {isWeb(href) ? (
        <a href={href} target="_blank" rel="noreferrer noopener" title={href} className={`${QUIET_LINK} inline-flex max-w-full items-center gap-1.5`}>
          <span className="truncate">{text}</span>
          <Icon name="external" className="size-3.5" />
          <span className="sr-only">{t("common.opensNewTab")}</span>
        </a>
      ) : (
        <span title={href} className="truncate">
          {text}
        </span>
      )}
    </p>
  );
}

function Skeleton() {
  return (
    <div aria-hidden="true" className="mt-6 flex w-full flex-col items-center lg:items-start">
      <div className="shimmer size-16 rounded-2xl" />
      <div className={`${SKELETON_LINE} mt-5 h-11 w-72 max-w-full`} />
      <div className={`${SKELETON_LINE} mt-5 h-5 w-56 max-w-full`} />
    </div>
  );
}

/** The head of the install-link page, centred below `lg` and left-aligned in its column from `lg`: a quiet caption naming where the link came from, then the icon tile, the one serif title, a row of chips, the description and a single provenance line; a skeleton while the link is read and the bare host when it cannot be. */
export function LandingHeader({ host, preview }: { host: string; preview: UseQueryResult<Preview> }) {
  const t = useT();
  return (
    <header className="flex min-w-0 flex-col items-center text-center lg:items-start lg:text-left">
      <p className={`${CAPTION} max-w-full truncate`} translate="no">
        {t("import.linkEyebrow", { host })}
      </p>
      {preview.isSuccess ? (
        <div className="swap-in flex min-w-0 max-w-full flex-col items-center lg:items-start">
          <div className="mt-6">
            <ConnectorIcon src={preview.data.icon} title={preview.data.title} size="lg" />
          </div>
          <Display className="mt-5 max-w-full [overflow-wrap:anywhere]">{preview.data.title}</Display>
          <div className="mt-5 max-w-full">
            <MetaChips preview={preview.data} />
          </div>
          {preview.data.description ? (
            <p title={preview.data.description} className="mt-5 line-clamp-4 max-w-[60ch] text-pretty text-muted">
              {preview.data.description}
            </p>
          ) : null}
          <Provenance preview={preview.data} host={host} />
        </div>
      ) : preview.isError ? (
        <Display className="mt-6 max-w-full [overflow-wrap:anywhere]">{host}</Display>
      ) : (
        <Skeleton />
      )}
    </header>
  );
}
