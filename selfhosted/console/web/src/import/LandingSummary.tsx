import type { ReactNode } from "react";
import type { Access, Preview, ProbedTool } from "../api";
import { AccessBar } from "../connectors/AccessBar";
import { accessCounts } from "../connectors/summary";
import { useT } from "../i18n/I18nProvider";
import { Icon } from "../ui/Icon";
import { TITLE } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { SkillAccess } from "./SkillChoice";

type ProposalProps = { tools: ProbedTool[]; accessOf(name: string, proposed: Access): Access };

function Proposal({ tools, accessOf }: ProposalProps) {
  const t = useT();
  const counts = accessCounts(tools.map((tool) => ({ access: accessOf(tool.name, tool.proposed) })));
  return (
    <>
      <AccessBar counts={counts} />
      <div className="mt-2.5 flex flex-wrap gap-x-3 gap-y-0.5">
        <Tag>{t("connectors.functions", { count: tools.length })}</Tag>
        {counts.on > 0 ? <Tag tone="paper">{t("access.count.on", { count: counts.on })}</Tag> : null}
        {counts.ask > 0 ? <Tag tone="lime">{t("access.count.ask", { count: counts.ask })}</Tag> : null}
        {counts.off > 0 ? <Tag>{t("access.count.off", { count: counts.off })}</Tag> : null}
      </div>
      <p className="mt-3 text-sm text-pretty text-muted">{t("import.proposedHint")}</p>
    </>
  );
}

type LandingSummaryProps = {
  preview: Preview | undefined;
  accessOf(name: string, proposed: Access): Access;
  skillAccess: Access;
  onSkillAccess(next: Access): void;
  actions: ReactNode;
};

/** The decision of an install link: who may load a skill, or the split of a connector's proposed access when it lists functions; always the notice that nothing is installed before the owner confirms, then the actions from `lg`, where the card stays beside the review. */
export function LandingSummary({ preview, accessOf, skillAccess, onSkillAccess, actions }: LandingSummaryProps) {
  const t = useT();
  const skill = preview?.skill;
  const body = !preview ? null : skill ? <SkillAccess skill={skill} access={skillAccess} onChange={onSkillAccess} /> : preview.tools.length > 0 ? <Proposal tools={preview.tools} accessOf={accessOf} /> : null;
  return (
    <div className="flex flex-col gap-5">
      {body ? (
        <section aria-labelledby="summary-title" className="swap-in">
          <h2 id="summary-title" className={TITLE}>
            {t(skill ? "import.access" : "import.proposed")}
          </h2>
          <div className="mt-4">{body}</div>
        </section>
      ) : null}
      <p className={`flex items-start gap-2 text-sm text-pretty text-muted ${body ? "border-t border-line pt-5" : ""}`}>
        <Icon name="shield" className="mt-0.5 size-4" />
        {t("import.untrusted")}
      </p>
      {actions ? <div className="hidden lg:block">{actions}</div> : null}
    </div>
  );
}
