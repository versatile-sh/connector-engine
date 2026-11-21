import type { Access, Preview } from "../api";
import { useT } from "../i18n/I18nProvider";
import { SkillFactsView } from "../skills/SkillFactsView";
import { Notice } from "../ui/Notice";
import { CAPTION, TITLE } from "../ui/styles";
import { Choices, ChoicesSkeleton } from "./Choices";
import { PreviewFacts } from "./Identity";

type LandingReviewProps = { preview: Preview; accessOf(name: string, proposed: Access): Access; choose(name: string, access: Access): void };

/** The review details of an install link: for a skill its warnings, licence, source at its commit, files and the table of AI apps; for a connector its server facts and every function with its proposed access, changeable before the install. */
export function LandingReview({ preview, accessOf, choose }: LandingReviewProps) {
  const t = useT();
  const { skill } = preview;
  return (
    <div className="swap-in">
      <h2 id="review-title" className={TITLE}>
        {t(skill ? "import.skillChoose" : "import.choose")}
      </h2>
      <p className="mt-1 text-sm text-pretty text-muted">{t(skill ? "import.skillReviewIntro" : "import.chooseIntro")}</p>
      <div className="mt-5 flex flex-col gap-5">
        {preview.warning ? <Notice>{preview.warning}</Notice> : null}
        {skill ? (
          <SkillFactsView skill={skill} />
        ) : (
          <>
            <PreviewFacts preview={preview} from={false} />
            <Choices tools={preview.tools} accessOf={accessOf} choose={choose} />
          </>
        )}
      </div>
    </div>
  );
}

/** The review while the link is read: what is being read and the shape of the rows to come. */
export function LandingReading({ url }: { url: string }) {
  const t = useT();
  return (
    <div role="status" aria-live="polite">
      <p className={`${CAPTION} mb-3 truncate`}>{t("import.reading", { url })}</p>
      <ChoicesSkeleton rows={5} />
    </div>
  );
}
