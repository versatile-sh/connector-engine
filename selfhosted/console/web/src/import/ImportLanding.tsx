import { errorText, type ImportSource } from "../api";
import { Frame } from "../Frame";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Notice } from "../ui/Notice";
import { PANEL, button } from "../ui/styles";
import { LandingActions } from "./LandingActions";
import { LandingHeader } from "./LandingHeader";
import { LandingReading, LandingReview } from "./LandingReview";
import { LandingSummary } from "./LandingSummary";
import { useImport } from "./useImport";

const hostOf = (raw: string) => {
  try {
    return new URL(raw).host;
  } catch {
    return raw;
  }
};

const FOOTER =
  "sticky bottom-0 z-20 -mx-5 -mb-10 border-t border-line bg-ink px-5 py-3 sm:-mx-8 sm:-mb-14 sm:px-8 lg:m-0 lg:rounded-b-2xl lg:border lg:bg-ink-raised lg:px-6 lg:py-4";

/** An install link opened from outside: the link's identity on the left from `lg`, and on the right one card as tall as its content, never scrolling inside itself, holding the review, who may load it and the notice, with the actions as its foot (stuck to the window's bottom only when a long list makes the card taller than the window); below `lg` one column, hero, review and decision in one panel, then the actions as a solid bar in the flow, so it never covers the end of the review. Nothing is installed before Add. */
export function ImportLanding({ source }: { source: ImportSource }) {
  const t = useT();
  const { preview, install, request, accessOf, choose, skillAccess, setSkillAccess } = useImport(source);
  const url = source.listing ?? source.mcp ?? "";
  const header = <LandingHeader host={hostOf(url)} preview={preview} />;
  if (preview.isError) {
    return (
      <Frame width="split" home>
        <div className="mx-auto flex w-full min-w-0 max-w-2xl flex-1 flex-col justify-start gap-10">
          {header}
          <section className={`${PANEL} flex w-full flex-col items-start gap-5 p-5 sm:p-6`}>
            <Notice>{errorText(preview.error)}</Notice>
            <button type="button" className={button({ variant: "secondary" })} onClick={() => navigate("/")}>
              {t("import.home")}
            </button>
          </section>
        </div>
      </Frame>
    );
  }
  const actions = preview.isSuccess ? (
    <LandingActions
      label={t(preview.data.skill ? "import.addSkill" : "import.addConnector")}
      pending={install.isPending}
      error={install.isError ? errorText(install.error) : undefined}
      onInstall={() => install.mutate(request, { onSuccess: ({ connector }) => navigate(connector ? `/connectors/${encodeURIComponent(connector)}` : "/skills") })}
      onCancel={() => navigate("/connectors")}
    />
  ) : null;
  return (
    <Frame width="split" home>
      <div className="flex w-full min-w-0 flex-1 flex-col justify-start gap-10 lg:grid lg:grid-cols-[minmax(0,1fr)_30rem] lg:content-start lg:items-start">
        {header}
        <div className="contents lg:flex lg:min-w-0 lg:flex-col">
          <div className={`${PANEL} min-w-0 p-5 sm:p-6 ${actions ? "lg:rounded-b-none lg:border-b-0" : ""}`}>
            {preview.isSuccess ? (
              <section aria-labelledby="review-title">
                <LandingReview preview={preview.data} accessOf={accessOf} choose={choose} />
              </section>
            ) : (
              <LandingReading url={url} />
            )}
            <LandingSummary preview={preview.data} accessOf={accessOf} skillAccess={skillAccess} onSkillAccess={setSkillAccess} />
          </div>
          {actions ? <div className={FOOTER}>{actions}</div> : null}
        </div>
      </div>
    </Frame>
  );
}
