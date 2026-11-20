import { errorText, type ImportSource } from "../api";
import { Wordmark } from "../Brand";
import { useT } from "../i18n/I18nProvider";
import { LanguageSwitch } from "../LanguageSwitch";
import { navigate } from "../router";
import { Split } from "../Split";
import { Icon, Arrow } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { button } from "../ui/styles";
import { Choices, ChoicesSkeleton } from "./Choices";
import { Identity } from "./Identity";
import { useImport } from "./useImport";

const hostOf = (raw: string) => {
  try {
    return new URL(raw).host;
  } catch {
    return raw;
  }
};

/** An install link opened from outside, as the live doorway: who the connector is on lime, what it may do on ink, and nothing installed before Install. */
export function ImportLanding({ source }: { source: ImportSource }) {
  const t = useT();
  const { preview, install, accessOf, choose } = useImport(source);
  const host = hostOf(source.listing ?? source.mcp ?? "");
  const home = (
    <a
      href="/"
      aria-label={t("app.home")}
      className="self-start"
      onClick={(event) => {
        event.preventDefault();
        navigate("/");
      }}
    >
      <Wordmark onAccent />
    </a>
  );
  return (
    <Split
      pitch={
        <>
          {home}
          <div className="flex flex-1 flex-col justify-center pt-8 pb-2 lg:py-0">
            <p className="bubble-word mb-6 inline-flex self-start bg-ink px-3 py-1 font-mono text-xs text-lime" translate="no">
              {t("import.linkEyebrow", { host })}
            </p>
            {preview.isSuccess ? (
              <div className="ride-in">
                <Identity preview={preview.data} onLime />
              </div>
            ) : (
              <div aria-hidden="true" className="flex items-center gap-4">
                <div className={`size-16 rounded-full bg-ink/15 ${preview.isPending ? "pulse" : ""}`} />
                <div className="flex-1">
                  <div className={`bubble-word h-10 w-64 max-w-full bg-ink/15 ${preview.isPending ? "pulse" : ""}`} />
                  <div className="bubble-word mt-3 h-3 w-40 bg-ink/15" />
                </div>
              </div>
            )}
            <p className="mt-8 flex max-w-md items-start gap-2 text-sm text-ink/70">
              <Icon name="shield" className="mt-0.5 size-4" />
              {t("import.untrusted")}
            </p>
          </div>
        </>
      }
      panel={
        <>
          <LanguageSwitch className="self-end" />
          <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col pt-6">
            <h1 className="text-lit font-serif text-4xl leading-none font-normal tracking-[-0.02em]">{t("import.choose")}</h1>
            <p className="mt-2 text-muted">{t("import.chooseIntro")}</p>
            <div className="mt-4 flex-1">
              {preview.isPending ? (
                <div role="status" aria-live="polite">
                  <p className="truncate font-mono text-xs text-muted">{t("import.reading", { url: source.listing ?? source.mcp ?? "" })}</p>
                  <ChoicesSkeleton rows={5} />
                </div>
              ) : preview.isError ? (
                <div className="flex flex-col gap-5 pt-2">
                  <Notice>{errorText(preview.error)}</Notice>
                  <button type="button" className={`${button({ variant: "secondary" })} self-start`} onClick={() => navigate("/")}>
                    {t("import.home")}
                  </button>
                </div>
              ) : (
                <div className="swap-in">
                  {preview.data.warning ? (
                    <div className="mb-3">
                      <Notice>{preview.data.warning}</Notice>
                    </div>
                  ) : null}
                  <Choices tools={preview.data.tools} accessOf={accessOf} choose={choose} />
                </div>
              )}
            </div>
            {preview.isSuccess ? (
              <div className="sticky bottom-0 mt-4 border-t border-line bg-ink/90 py-4 backdrop-blur-md">
                <p role="alert" className="min-h-5 text-sm text-amber">
                  {install.isError ? errorText(install.error) : ""}
                </p>
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <button type="button" className={button({ variant: "quiet" })} onClick={() => navigate("/")}>
                    {t("common.cancel")}
                  </button>
                  <button
                    type="button"
                    disabled={install.isPending}
                    className={button({ variant: "accent", size: "lg" })}
                    onClick={() => install.mutate(undefined, { onSuccess: ({ connector }) => navigate(`/connectors/${encodeURIComponent(connector)}`) })}
                  >
                    {install.isPending ? t("import.installing") : t("import.install", { count: preview.data.tools.length })}
                    <Arrow />
                  </button>
                </div>
              </div>
            ) : null}
          </div>
        </>
      }
    />
  );
}
