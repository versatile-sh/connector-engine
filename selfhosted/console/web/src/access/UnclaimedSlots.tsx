import { useQuery } from "@tanstack/react-query";
import { useT } from "../i18n/I18nProvider";
import { slotsQuery } from "../queries";
import { SUBTITLE, SURFACE, WELL } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { useKept } from "../ui/useKept";
import { SlotRow } from "./SlotRow";

/** The permission sets an imported setup left for keys to claim, each with what it holds and the way to apply or discard it; nothing when there is none. */
export function UnclaimedSlots() {
  const t = useT();
  const slots = useKept(useQuery(slotsQuery).data ?? [], (slot) => slot.name);
  if (slots.length === 0) return null;
  return (
    <section aria-labelledby="slots-title" className={`${SURFACE} arrive p-5`}>
      <div className="flex items-baseline gap-3">
        <h2 id="slots-title" className={SUBTITLE}>
          {t("slots.title")}
        </h2>
        <Tag>{slots.length}</Tag>
      </div>
      <p className="mt-1 mb-4 max-w-2xl text-sm text-pretty text-muted">{t("slots.intro")}</p>
      <ul className={`${WELL} @container`}>
        {slots.map(({ row: slot, phase }) => (
          <SlotRow key={slot.name} name={slot.name} profiles={slot.connectors} phase={phase} />
        ))}
      </ul>
    </section>
  );
}
