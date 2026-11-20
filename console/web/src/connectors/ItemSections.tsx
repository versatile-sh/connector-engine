import { useId, useState, type ReactNode } from "react";
import type { Access, Item, ItemKind } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { ACCENT_BAR, QUIET_LINK, SURFACE, TOUCH, button } from "../ui/styles";
import { AccessPicker } from "./AccessPicker";
import { ITEM_KINDS, accessChoices, groupItems } from "./items";

type ItemSectionsProps = {
  items: readonly Item[] | null | undefined;
  variant: "page" | "review";
  accessOf(item: Item): Access;
  pendingOf?(item: Item): boolean;
  choose(item: Item, access: Access): void;
};

const TEXT_BOX = "rounded-lg border border-line bg-ink font-mono text-xs leading-relaxed whitespace-pre-wrap text-paper [overflow-wrap:anywhere]";

function StateFlag({ item }: { item: Item }) {
  const t = useT();
  if (!item.state || item.state === "active") return null;
  return (
    <Bubble tone="amber" title={item.state === "quarantined" ? t("detail.quarantined") : undefined}>
      {item.state}
    </Bubble>
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
    <section aria-labelledby={id} className={variant === "page" ? `${SURFACE} swap-in min-w-0 p-5 sm:p-6` : "swap-in mt-7 min-w-0"}>
      <div className="flex items-center gap-2.5">
        <Heading id={id} className={variant === "page" ? "text-lg leading-snug font-medium" : "text-base font-medium"}>
          {t(`items.${kind}.title`)}
        </Heading>
        {kind === "instructions" ? null : <Bubble>{count}</Bubble>}
      </div>
      <p className="mt-1 text-sm text-muted">{t(`items.${kind}.intro`)}</p>
      {children}
    </section>
  );
}

function Instructions({ item, picker, accept }: { item: Item; picker: ReactNode; accept: ReactNode }) {
  const t = useT();
  return (
    <div className="@container mt-4 flex flex-col gap-3">
      <pre tabIndex={0} aria-label={t("items.instructions.text")} className={`${TEXT_BOX} max-h-80 overflow-auto p-4`}>
        {item.preview || t("items.noText")}
      </pre>
      {item.state === "quarantined" ? (
        <div className="arrive bubble relative border border-line-strong bg-ink-raised py-3 pr-3 pl-5 text-sm">
          <span aria-hidden="true" className={`${ACCENT_BAR} bg-amber`} />
          <p className="[overflow-wrap:anywhere]">{t("items.instructions.quarantined")}</p>
        </div>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <StateFlag item={item} />
          {accept}
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
      <div id={id} inert={!open} className={`grid transition-[grid-template-rows,opacity] duration-250 ease-(--ease-glide) ${open ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"}`}>
        <div className="overflow-hidden">
          <pre tabIndex={0} className={`${TEXT_BOX} mt-2 max-h-64 overflow-auto p-3`}>
            {text}
          </pre>
        </div>
      </div>
    </>
  );
}

function ItemRow({ item, picker, accept }: { item: Item; picker: ReactNode; accept: ReactNode }) {
  const t = useT();
  return (
    <li className="flex flex-col gap-3 border-b border-line py-3.5 last:border-b-0 @lg:flex-row @lg:items-start @lg:gap-6">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <code className="min-w-0 font-mono text-sm font-semibold break-all text-paper" translate="no">
            {item.label || item.name}
          </code>
          {item.title && item.title !== item.label ? <span className="text-sm text-muted [overflow-wrap:anywhere]">{item.title}</span> : null}
          {item.template ? (
            <Bubble tone="outline" title={t("items.templateHint")}>
              {t("items.template")}
            </Bubble>
          ) : null}
          <StateFlag item={item} />
          {accept}
        </div>
        {item.kind === "prompt" ? (
          <PromptPreview text={item.preview} />
        ) : item.preview ? (
          <p className="mt-1 text-sm whitespace-pre-line text-muted [overflow-wrap:anywhere]">{item.preview}</p>
        ) : null}
      </div>
      {picker}
    </li>
  );
}

/** A connector's server instructions, prompts and resources in one section per kind that has any, each item with its text, its access choice and, when quarantined while on or ask, a button that accepts the new text. */
export function ItemSections({ items, variant, accessOf, pendingOf, choose }: ItemSectionsProps) {
  const t = useT();
  const groups = groupItems(items);
  const pickerOf = (item: Item) => (
    <AccessPicker
      name={item.label || t("items.instructions.title")}
      of="item"
      choices={accessChoices(item.kind)}
      value={accessOf(item)}
      disabled={pendingOf?.(item) ?? false}
      onChange={(access) => choose(item, access)}
    />
  );
  const acceptOf = (item: Item) => <Accept item={item} access={accessOf(item)} disabled={pendingOf?.(item) ?? false} choose={choose} />;
  return (
    <>
      {ITEM_KINDS.map((kind) =>
        groups[kind].length === 0 ? null : (
          <Section key={kind} kind={kind} count={groups[kind].length} variant={variant}>
            {kind === "instructions" ? (
              groups[kind].map((item) => <Instructions key={item.name} item={item} picker={pickerOf(item)} accept={acceptOf(item)} />)
            ) : (
              <ul className="@container mt-2">
                {groups[kind].map((item) => (
                  <ItemRow key={item.name} item={item} picker={pickerOf(item)} accept={acceptOf(item)} />
                ))}
              </ul>
            )}
          </Section>
        ),
      )}
    </>
  );
}
