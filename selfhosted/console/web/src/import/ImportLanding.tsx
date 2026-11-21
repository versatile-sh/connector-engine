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

/** An install link opened from outside, in one centred composition: the link's identity on top, then from `lg` two cards side by side, the review on the left and a sticky decision card on the right that holds who may load it, the notice and the actions; below `lg` one column that ends in a solid bar of actions in the flow, so it never covers the end of the review. Nothing is installed before Add. */
export function ImportLanding({ source }: { source: ImportSource }) {
  const t = useT();
  const { preview, install, request, accessOf, choose, skillAccess, setSkillAccess } = useImport(source);
  const url = source.listing ?? source.mcp ?? "";
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
      <div className="flex w-full min-w-0 flex-1 flex-col justify-start gap-10">
        <LandingHeader host={hostOf(url)} preview={preview} />
        {preview.isError ? (
          <section className={`${PANEL} mx-auto flex w-full max-w-2xl flex-col items-start gap-5 p-5 sm:p-6`}>
            <Notice>{errorText(preview.error)}</Notice>
            <button type="button" className={button({ variant: "secondary" })} onClick={() => navigate("/")}>
              {t("import.home")}
            </button>
          </section>
        ) : (
          <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_26rem] lg:gap-10">
            <section aria-labelledby={preview.isSuccess ? "review-title" : undefined} className={`${PANEL} min-w-0 p-5 sm:p-6`}>
              {preview.isSuccess ? <LandingReview preview={preview.data} accessOf={accessOf} choose={choose} /> : <LandingReading url={url} />}
            </section>
            <div className={`${PANEL} p-5 sm:p-6 lg:sticky lg:top-6`}>
              <LandingSummary preview={preview.data} accessOf={accessOf} skillAccess={skillAccess} onSkillAccess={setSkillAccess} actions={actions} />
            </div>
          </div>
        )}
        {actions ? <div className="sticky bottom-0 z-20 -mx-5 -mb-10 border-t border-line bg-ink px-5 py-3 sm:-mx-8 sm:-mb-14 sm:px-8 lg:hidden">{actions}</div> : null}
      </div>
    </Frame>
  );
}
