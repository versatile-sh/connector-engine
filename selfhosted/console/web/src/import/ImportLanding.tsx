import { errorText, type ImportSource } from "../api";
import { Frame } from "../Frame";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { Arrow, Icon } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { CAPTION, PANEL, SKELETON_LINE, TITLE, button } from "../ui/styles";
import { Choices, ChoicesSkeleton } from "./Choices";
import { Identity } from "./Identity";
import { SkillChoice } from "./SkillChoice";
import { useImport } from "./useImport";

const hostOf = (raw: string) => {
  try {
    return new URL(raw).host;
  } catch {
    return raw;
  }
};

/** An install link opened from outside, as the site's connector page: who the connector is in the headline, what it may do in one panel, and nothing installed before Install. */
export function ImportLanding({ source }: { source: ImportSource }) {
  const t = useT();
  const { preview, install, request, accessOf, choose, skillAccess, setSkillAccess } = useImport(source);
  const host = hostOf(source.listing ?? source.mcp ?? "");
  return (
    <Frame width="wide" home>
      <div className="flex w-full flex-1 flex-col justify-start">
        <p className={`${CAPTION} [overflow-wrap:anywhere]`} translate="no">
          {t("import.linkEyebrow", { host })}
        </p>
        <header className="mt-6 mb-10">
          {preview.isSuccess ? (
            <div className="swap-in">
              <Identity preview={preview.data} as="h1" />
            </div>
          ) : (
            <div aria-hidden="true" className="flex items-center gap-5">
              <div className="shimmer size-16 rounded-2xl" />
              <div className="flex-1">
                <div className={`${SKELETON_LINE} h-11 w-72 max-w-full`} />
                <div className={`${SKELETON_LINE} mt-3 h-3 w-40`} />
              </div>
            </div>
          )}
          <p className="mt-6 flex max-w-xl items-start gap-2 text-sm text-pretty text-muted">
            <Icon name="shield" className="mt-0.5 size-4" />
            {t("import.untrusted")}
          </p>
        </header>
        <section aria-labelledby="choose-title" className={`${PANEL} p-4 sm:p-8`}>
          <h2 id="choose-title" className={TITLE}>
            {t(preview.data?.skill ? "import.skillChoose" : "import.choose")}
          </h2>
          <p className="mt-1 text-sm text-pretty text-muted">{t(preview.data?.skill ? "import.skillChooseIntro" : "import.chooseIntro")}</p>
          <div className="mt-5">
            {preview.isPending ? (
              <div role="status" aria-live="polite">
                <p className={`${CAPTION} mb-3 truncate`}>{t("import.reading", { url: source.listing ?? source.mcp ?? "" })}</p>
                <ChoicesSkeleton rows={5} />
              </div>
            ) : preview.isError ? (
              <div className="flex flex-col items-start gap-5">
                <Notice>{errorText(preview.error)}</Notice>
                <button type="button" className={button({ variant: "secondary" })} onClick={() => navigate("/")}>
                  {t("import.home")}
                </button>
              </div>
            ) : (
              <div className="swap-in">
                {preview.data.warning ? <Notice className="mb-4">{preview.data.warning}</Notice> : null}
                {preview.data.skill ? <SkillChoice skill={preview.data.skill} access={skillAccess} onChange={setSkillAccess} /> : <Choices tools={preview.data.tools} accessOf={accessOf} choose={choose} />}
              </div>
            )}
          </div>
        </section>
        {preview.isSuccess ? (
          <div className="sticky bottom-0 z-20 mt-6 border-t border-line bg-ink/90 py-4 backdrop-blur-md">
            <p role="alert" className="min-h-5 text-sm text-amber-400">
              {install.isError ? errorText(install.error) : ""}
            </p>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <button type="button" className={button({ variant: "quiet" })} onClick={() => navigate("/")}>
                {t("common.cancel")}
              </button>
              <button
                type="button"
                disabled={install.isPending}
                className={button()}
                onClick={() => install.mutate(request, { onSuccess: ({ connector }) => navigate(connector ? `/connectors/${encodeURIComponent(connector)}` : "/skills") })}
              >
                {install.isPending ? t("import.installing") : preview.data.skill ? t("import.addSkill") : t("import.install", { count: preview.data.tools.length })}
                <Arrow />
              </button>
            </div>
          </div>
        ) : null}
      </div>
    </Frame>
  );
}
