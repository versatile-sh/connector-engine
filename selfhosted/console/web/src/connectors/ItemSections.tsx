import { useId, useState, type ReactNode } from "react";
import type { Access, Item, ItemKind } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Collapse } from "../ui/Collapse";
import { Notice } from "../ui/Notice";
import { QUIET_LINK, SUBTITLE, TITLE, TOUCH, WELL, button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { AccessPicker } from "./AccessPicker";
import { ChoiceRow } from "./ChoiceRow";
import { ITEM_KINDS, accessChoices, groupItems } from "./items";

type ItemSectionsProps = {
  items: readonly Item[] | null | undefined;
  variant: "page" | "review";
  accessOf(item: Item): Access;
  pendingOf?(item: Item): boolean;
  mostOf?(item: Item): Access | undefined;
  flagsOf?(item: Item): ReactNode;
  accepts?: boolean;
  choose(item: Item, access: Access): void;
};

const TEXT_BOX = "rounded-lg border border-line bg-ink font-mono text-xs leading-relaxed whitespace-pre-wrap text-paper [overflow-wrap:anywhere]";

function StateFlag({ item }: { item: Item }) {
  const t = useT();
  if (!item.state || item.state === "active") return null;
  return (
    <Tag tone="amber" dot title={item.state === "quarantined" ? t("detail.quarantined") : undefined}>
      {item.state}
    </Tag>
  );
}

function Accept({ item, access, disabled, choose }: { item: Item; access: Access; disabled: boolean; choose(item: Item, access: Access): void }) {
  const t = useT();
  if (item.state !== "quarantined" || access === "off") return null;
  return (
    <button type="button" disabled={disabled} className={`${button({ variant: "secondary", size: "xs" })} arrive`} onClick={() => choose(item, access)}>
      {t("items.accept")}
    </button>
  );
}

function Section({ kind, count, variant, children }: { kind: ItemKind; count: number; variant: ItemSectionsProps["variant"]; children: ReactNode }) {
  const t = useT();
  const id = useId();
  const Heading = variant === "page" ? "h2" : "h3";
  return (
    <section aria-labelledby={id} className={variant === "page" ? `${WELL} swap-in min-w-0` : "swap-in mt-8 min-w-0"}>
      <div className={variant === "page" ? "px-4 pt-4 sm:px-5" : ""}>
        <div className="flex flex-wrap items-baseline gap-x-4">
          <Heading id={id} className={variant === "page" ? TITLE : SUBTITLE}>
            {t(`items.${kind}.title`)}
          </Heading>
          {kind === "instructions" ? null : <Tag>{count}</Tag>}
        </div>
        <p className="mt-1 text-sm text-pretty text-muted">{t(`items.${kind}.intro`)}</p>
      </div>
      {children}
    </section>
  );
}

function Instructions({ item, picker, accept, flags }: { item: Item; picker: ReactNode; accept: ReactNode; flags: ReactNode }) {
  const t = useT();
  return (
    <div className="@container mt-4 flex flex-col gap-3">
      <pre tabIndex={0} aria-label={t("items.instructions.text")} className={`${TEXT_BOX} max-h-80 overflow-auto p-4`}>
        {item.preview || t("items.noText")}
      </pre>
      {item.state === "quarantined" ? <Notice live={false}>{t("items.instructions.quarantined")}</Notice> : null}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <StateFlag item={item} />
          {accept}
          {flags}
        </div>
        <div className="ml-auto w-full @lg:w-auto">{picker}</div>
      </div>
    </div>
  );
}

function PromptPreview({ text }: { text: string }) {
  const t = useT();
  const id = useId();
  const [open, setOpen] = useState(false);
  if (!text) return null;
  return (
    <>
      <button type="button" aria-expanded={open} aria-controls={id} onClick={() => setOpen((was) => !was)} className={`${QUIET_LINK} relative mt-1.5 text-xs text-muted ${TOUCH}`}>
        {t(open ? "items.hidePreview" : "items.showPreview")}
      </button>
      <Collapse id={id} open={open}>
        <pre tabIndex={0} className={`${TEXT_BOX} mt-2 max-h-64 overflow-auto p-3`}>
          {text}
        </pre>
      </Collapse>
    </>
  );
}

function ItemRow({ item, picker, accept, flags }: { item: Item; picker: ReactNode; accept: ReactNode; flags: ReactNode }) {
  const t = useT();
  return (
    <ChoiceRow
      name={item.label || item.name}
      title={item.title}
      flags={
        <>
          {item.template ? (
            <Tag title={t("items.templateHint")}>{t("items.template")}</Tag>
          ) : null}
          <StateFlag item={item} />
          {accept}
          {flags}
        </>
      }
      description={item.kind === "prompt" ? <PromptPreview text={item.preview} /> : item.preview ? <p className="mt-1 text-sm whitespace-pre-line text-muted">{item.preview}</p> : null}
    >
      {picker}
    </ChoiceRow>
  );
}

/** A connector's server instructions, prompts and resources in one section per kind that has any (on a page, each one card with its heading inside), each item with its text, its access choice (at most `mostOf`), the caller's flags and, when it `accepts` and the item is quarantined while on or ask, a button that accepts the new text. */
export function ItemSections({ items, variant, accessOf, pendingOf, mostOf, flagsOf, accepts = true, choose }: ItemSectionsProps) {
  const t = useT();
  const groups = groupItems(items);
  const pickerOf = (item: Item) => (
    <AccessPicker
      name={item.label || t("items.instructions.title")}
      of="item"
      choices={accessChoices(item.kind)}
      value={accessOf(item)}
      most={mostOf?.(item)}
      disabled={pendingOf?.(item) ?? false}
      onChange={(access) => choose(item, access)}
    />
  );
  const acceptOf = (item: Item) => (accepts ? <Accept item={item} access={accessOf(item)} disabled={pendingOf?.(item) ?? false} choose={choose} /> : null);
  return (
    <>
      {ITEM_KINDS.map((kind) =>
        groups[kind].length === 0 ? null : (
          <Section key={kind} kind={kind} count={groups[kind].length} variant={variant}>
            {kind === "instructions" ? (
              <div className={variant === "page" ? "px-4 pb-4 sm:px-5" : ""}>
                {groups[kind].map((item) => (
                  <Instructions key={item.name} item={item} picker={pickerOf(item)} accept={acceptOf(item)} flags={flagsOf?.(item)} />
                ))}
              </div>
            ) : (
              <ul className={variant === "page" ? "mt-3 border-t border-line" : `${WELL} mt-3`}>
                {groups[kind].map((item) => (
                  <ItemRow key={item.name} item={item} picker={pickerOf(item)} accept={acceptOf(item)} flags={flagsOf?.(item)} />
                ))}
              </ul>
            )}
          </Section>
        ),
      )}
    </>
  );
}
