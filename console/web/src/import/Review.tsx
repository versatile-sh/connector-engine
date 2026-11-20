import type { ImportSource } from "../api";
import { errorText } from "../api";
import { useT } from "../i18n/I18nProvider";
import { navigate } from "../router";
import { ItemSections } from "../connectors/ItemSections";
import { Bubble } from "../ui/Bubble";
import { Arrow } from "../ui/Icon";
import { Notice } from "../ui/Notice";
import { button } from "../ui/styles";
import { Choices, ChoicesSkeleton } from "./Choices";
import { Identity } from "./Identity";
import { SkillChoice } from "./SkillChoice";
import { useImport } from "./useImport";

/** The second step of the import window: what the server is, the proposed access of each function, prompt, resource and the server instructions, and Install. */
export function Review({ source, onBack, onInstalled }: { source: ImportSource; onBack(): void; onInstalled(): void }) {
  const t = useT();
  const { preview, install, accessOf, choose, skillAccess, setSkillAccess } = useImport(source);
  const back = (
    <button type="button" className={button({ variant: "quiet", size: "sm" })} onClick={onBack}>
      <Arrow back />
      {t("import.back")}
    </button>
  );
  if (preview.isPending) {
    return (
      <div role="status" aria-live="polite">
        <p className="mb-4 truncate font-mono text-xs text-muted">{t("import.reading", { url: source.listing ?? source.mcp ?? "" })}</p>
        <div className="flex items-center gap-4">
          <div className="shimmer size-11 rounded-full" />
          <div className="flex-1">
            <div className="shimmer bubble-word h-6 w-48" />
            <div className="shimmer bubble-word mt-2 h-3 w-32" />
          </div>
        </div>
        <ChoicesSkeleton />
      </div>
    );
  }
  if (preview.isError) {
    return (
      <div className="flex flex-col gap-5">
        <Notice>{errorText(preview.error)}</Notice>
        <div>{back}</div>
      </div>
    );
  }
  const p = preview.data;
  return (
    <div className="swap-in">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <Identity preview={p} />
        {source.local ? <Bubble tone="outline">{t("import.localTick")}</Bubble> : null}
      </div>
      {p.warning ? (
        <div className="mt-5">
          <Notice>{p.warning}</Notice>
        </div>
      ) : null}
      <h3 className="mt-7 text-base font-medium">{t(p.skill ? "import.skillChoose" : "import.choose")}</h3>
      <p className="text-sm text-muted">{t(p.skill ? "import.skillChooseIntro" : "import.chooseIntro")}</p>
      <div className="mt-2">
        {p.skill ? <SkillChoice skill={p.skill} access={skillAccess} onChange={setSkillAccess} /> : <Choices tools={p.tools} accessOf={accessOf} choose={choose} />}
      </div>
      {p.skill ? null : <ItemSections items={p.items} variant="review" accessOf={(item) => accessOf(item.name, item.access)} choose={(item, access) => choose(item.name, access)} />}
      <p role="alert" className="min-h-6 text-sm text-amber">
        {install.isError ? errorText(install.error) : ""}
      </p>
      <div className="sticky bottom-0 z-20 -mx-6 -mb-6 flex flex-wrap items-center justify-between gap-3 border-t border-line bg-ink-raised/95 px-6 py-4 backdrop-blur-md">
        {back}
        <button
          type="button"
          disabled={install.isPending}
          className={button({ variant: "accent" })}
          onClick={() =>
            install.mutate(undefined, {
              onSuccess: ({ connector }) => {
                onInstalled();
                navigate(connector ? `/connectors/${encodeURIComponent(connector)}` : "/skills");
              },
            })
          }
        >
          {install.isPending ? t("import.installing") : p.skill ? t("import.addSkill") : t("import.install", { count: p.tools.length })}
          <Arrow />
        </button>
      </div>
    </div>
  );
}
