import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type SlotProfile } from "../api";
import { modeText } from "../connectors/profiles";
import { useT } from "../i18n/I18nProvider";
import { claimSlot, discardSlot } from "../mutations";
import { keysQuery } from "../queries";
import { InlineConfirm } from "../ui/InlineConfirm";
import { rowMotion, type RowPhase } from "../ui/kept";
import { Select } from "../ui/Select";
import { Tag } from "../ui/Tag";
import { button } from "../ui/styles";

type SlotTagsProps = { profiles: readonly SlotProfile[]; withAlias?: boolean };

/** What a permission set holds, one tag per connector: its mode there and how many rows it narrows, after the connector's name when `withAlias`. */
export function SlotTags({ profiles, withAlias = true }: SlotTagsProps) {
  const t = useT();
  return (
    <span className="flex min-w-0 flex-wrap gap-1.5">
      {profiles.map((p) => (
        <Tag key={p.connector} tone={p.mode === "custom" ? "paper" : "muted"} dot={p.mode === "off"}>
          {withAlias ? `${p.alias} · ${modeText(p, t)}` : modeText(p, t)}
        </Tag>
      ))}
    </span>
  );
}

function SlotActions({ name }: { name: string }) {
  const t = useT();
  const keys = useQuery(keysQuery);
  const [key, setKey] = useState<string>();
  const [asking, setAsking] = useState(false);
  const claim = useMutation(claimSlot);
  const discard = useMutation(discardSlot);
  const busy = claim.isPending || discard.isPending;
  const options = (keys.data?.agents ?? []).map((agent) => ({ value: agent.name, text: agent.name }));
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {options.length > 0 ? (
          <>
            <Select<string> label={t("slots.applyLabel", { set: name })} value={key} options={options} placeholder={t("slots.applyTo")} onChange={setKey} disabled={busy} className="w-48 max-w-full" />
            <button type="button" disabled={!key || busy} className={button({ variant: "secondary", size: "xs" })} onClick={() => key && claim.mutate({ slot: name, key })}>
              {claim.isPending ? t("slots.applying") : t("slots.apply")}
            </button>
          </>
        ) : null}
        {asking ? null : (
          <button type="button" disabled={busy} aria-label={t("slots.discardLabel", { set: name })} className={button({ variant: "danger", size: "xs" })} onClick={() => setAsking(true)}>
            {t("slots.discard")}
          </button>
        )}
      </div>
      {asking ? (
        <InlineConfirm
          label={t("slots.discardLabel", { set: name })}
          question={t("slots.discardAsk", { set: name })}
          confirm={t("slots.discard")}
          working={t("slots.discarding")}
          busy={discard.isPending}
          error={discard.isError ? errorText(discard.error) : undefined}
          onCancel={() => setAsking(false)}
          onConfirm={() => discard.mutate(name)}
        />
      ) : null}
      <p role="alert" className="mt-1 text-sm text-amber-400 empty:hidden">
        {claim.isError ? errorText(claim.error) : ""}
      </p>
    </>
  );
}

type SlotRowProps = { name: string; profiles: readonly SlotProfile[]; phase: RowPhase; withAlias?: boolean };

/** One unclaimed permission set, coming in or going out as `phase` says: its name, what it holds, and the way to apply it to an access key made already or discard it. */
export function SlotRow({ name, profiles, phase, withAlias = true }: SlotRowProps) {
  return (
    <li {...rowMotion(phase)} className="flex flex-col gap-2 border-b border-line px-4 py-3 last:border-b-0 sm:px-5">
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
        <code className="min-w-0 font-mono text-sm font-medium [overflow-wrap:anywhere]" translate="no">
          {name}
        </code>
        <SlotTags profiles={profiles} withAlias={withAlias} />
      </div>
      <SlotActions name={name} />
    </li>
  );
}
