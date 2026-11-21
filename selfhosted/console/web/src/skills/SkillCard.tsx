import { useRef } from "react";
import type { Skill } from "../api";
import { Icon } from "../ui/Icon";
import { LIT_CARD, SKELETON_LINE, SURFACE } from "../ui/styles";
import { SkillDialog } from "./SkillDialog";
import { SkillOverview } from "./SkillOverview";
import { rowMotion, type RowPhase } from "../ui/kept";

type SkillCardProps = { skill: Skill; href: string; open: boolean; phase?: RowPhase; onOpen(): void; onClose(): void };

/** One installed skill as a card of the grid that opens it: who made it, two lines of what it does and how its keys are set; open, it grows into a dialog over the page and shrinks back on close. */
export function SkillCard({ skill, href, open, phase, onOpen, onClose }: SkillCardProps) {
  const card = useRef<HTMLAnchorElement>(null);
  return (
    <li {...rowMotion(phase)}>
      <a
        ref={card}
        href={href}
        aria-haspopup="dialog"
        onClick={(event) => {
          event.preventDefault();
          onOpen();
        }}
        className={`group ${LIT_CARD} flex h-full flex-col p-5`}
      >
        <div className="flex items-start gap-3">
          <h2 className="min-w-0 flex-1 truncate leading-snug font-medium">{skill.title}</h2>
          <Icon name="expand" className="mt-0.5 size-4 text-muted transition-colors duration-250 group-hover:text-lime group-focus-visible:text-lime" />
        </div>
        <SkillOverview skill={skill} compact />
      </a>
      {open ? <SkillDialog skill={skill} origin={card} onClose={onClose} /> : null}
    </li>
  );
}

/** The shape of a skill card while the list loads. */
export function SkillCardSkeleton() {
  return (
    <li aria-hidden="true" className={`${SURFACE} p-5`}>
      <div className={`${SKELETON_LINE} h-5 w-40`} />
      <div className={`${SKELETON_LINE} mt-3 h-3 w-28`} />
      <div className={`${SKELETON_LINE} mt-3 h-3 w-full`} />
      <div className={`${SKELETON_LINE} mt-2 h-3 w-2/3`} />
      <div className={`${SKELETON_LINE} mt-5 h-1.5 w-full`} />
    </li>
  );
}
