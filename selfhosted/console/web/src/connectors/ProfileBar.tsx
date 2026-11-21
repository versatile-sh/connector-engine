import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { errorText, type Connector, type Profile, type ProfileMode } from "../api";
import { useT } from "../i18n/I18nProvider";
import { SlotRow } from "../access/SlotRow";
import { setNewKeysOff, setProfileMode } from "../mutations";
import { navigate } from "../router";
import { Choice } from "../ui/Choice";
import { InlineConfirm } from "../ui/InlineConfirm";
import { Switch } from "../ui/Switch";
import { CAPTION } from "../ui/styles";
import { Tabs } from "../ui/Tabs";
import { Tag } from "../ui/Tag";
import { useKept } from "../ui/useKept";
import { commonMode, keysWithOverrides, overrideCount, profileHref } from "./profiles";

const MODES: readonly ProfileMode[] = ["off", "follow", "custom"];

/** The tabs of a connector's profiles, the default every key follows and each key's own, kept in the address as `?profile=`, controlling the panel `panel`. */
export function ProfileTabs({ connector, agent, panel, className }: { connector: Connector; agent: string | undefined; panel: string; className?: string }) {
  const t = useT();
  const options = [
    { value: "", text: t("profile.default"), hint: t("profile.defaultHint") },
    ...connector.profiles.map((p) => ({ value: p.agent, text: p.expired ? t("profile.expiredKey", { key: p.agent }) : p.agent, hint: t(`profile.modeHint.${p.mode}`), mono: true })),
  ];
  return <Tabs<string> label={t("profile.pickerLabel", { connector: connector.title })} value={agent ?? ""} options={options} onChange={(next) => navigate(profileHref(connector.id, next || undefined), { replace: true, keepScroll: true })} panel={panel} className={className} />;
}

function SetsHere({ connector }: { connector: Connector }) {
  const t = useT();
  const sets = useKept(connector.slots, (p) => p.slot);
  if (sets.length === 0) return null;
  return (
    <div className="py-3">
      <p className="px-4 text-sm font-medium sm:px-5">{t("profile.sets")}</p>
      <p className="px-4 text-[0.8125rem] leading-snug text-pretty text-muted sm:px-5">{t("profile.setsHint")}</p>
      <ul className="mt-1">
        {sets.map(({ row: p, phase }) => (
          <SlotRow key={p.slot} name={p.slot} profiles={[p]} phase={phase} withAlias={false} />
        ))}
      </ul>
    </div>
  );
}

/** The default's own controls: one choice that sets every key to off or the default in one request, whether keys made from now on start off, and the permission sets that hold a profile here, to apply to a key; without `manage`, only that admins set it, and the sets. */
export function DefaultBar({ connector, manage = true }: { connector: Connector; manage?: boolean }) {
  const t = useT();
  const every = useMutation(setProfileMode);
  const newKeys = useMutation(setNewKeysOff);
  const [asking, setAsking] = useState<"off" | "follow">();
  const losing = keysWithOverrides(connector.profiles);
  const setEvery = (mode: "off" | "follow") => every.mutate({ connector: connector.id, mode }, { onSuccess: () => setAsking(undefined) });
  const count = (mode: ProfileMode) => connector.profiles.filter((p) => p.mode === mode).length;
  const kept = every.data?.keys.filter((key) => key.error) ?? [];
  const error = every.isError && !asking ? errorText(every.error) : newKeys.isError ? errorText(newKeys.error) : kept.length > 0 ? t("profile.everyKeyKept", { keys: kept.map((key) => key.agent).join(", ") }) : "";
  if (!manage) {
    return (
      <div className="@container divide-y divide-line">
        <p className="px-4 py-3 text-sm text-pretty text-muted sm:px-5">{t("profile.byAdmins")}</p>
        <SetsHere connector={connector} />
      </div>
    );
  }
  return (
    <div className="@container divide-y divide-line">
      {connector.profiles.length === 0 ? <p className="px-4 py-3 text-sm text-pretty text-muted sm:px-5">{t("profile.noKeys")}</p> : null}
      {connector.profiles.length > 1 ? (
        <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 px-4 py-3 sm:px-5">
          <div className="min-w-0">
            <p className="text-sm font-medium">{t("profile.everyKey")}</p>
            <p className={CAPTION}>{t("profile.everyKeyCounts", { follow: count("follow"), custom: count("custom"), off: count("off") })}</p>
          </div>
          <Choice<"off" | "follow">
            label={t("profile.everyKeyLabel", { connector: connector.title })}
            disabled={every.isPending}
            value={every.isPending && every.variables.mode !== "custom" ? every.variables.mode : commonMode(connector.profiles)}
            options={(["off", "follow"] as const).map((mode) => ({ value: mode, text: t(`profile.mode.${mode}`), hint: t(`profile.everyKeyHint.${mode}`) }))}
            onChange={(mode) => (losing > 0 ? setAsking(mode) : setEvery(mode))}
          />
          {asking ? (
            <div className="w-full">
              <InlineConfirm
                label={t("profile.everyKeyLabel", { connector: connector.title })}
                question={t("profile.dropAllAsk", { count: losing, mode: t(`profile.mode.${asking}`) })}
                confirm={t("profile.dropConfirm")}
                working={t("profile.dropping")}
                busy={every.isPending}
                error={every.isError ? errorText(every.error) : undefined}
                onCancel={() => setAsking(undefined)}
                onConfirm={() => setEvery(asking)}
              />
            </div>
          ) : null}
        </div>
      ) : null}
      <div className="px-4 py-3 sm:px-5">
        <Switch checked={newKeys.isPending ? newKeys.variables.off : (connector.new_keys_off ?? false)} onChange={(off) => newKeys.mutate({ connector: connector.id, off })} hint={t("profile.newKeysOffHint")}>
          {t("profile.newKeysOff")}
        </Switch>
      </div>
      <SetsHere connector={connector} />
      <p role="alert" className="px-4 py-2 text-sm text-amber-400 empty:hidden sm:px-5">
        {error}
      </p>
    </div>
  );
}

type KeyBarProps = { connector: Connector; profile: Profile; mode: ProfileMode; onCustom(): void; onLeave(): void };

/** One key's mode on the connector, off, the default or custom, with what that means and how many rows it narrows. */
export function KeyBar({ connector, profile, mode, onCustom, onLeave }: KeyBarProps) {
  const t = useT();
  const set = useMutation(setProfileMode);
  const [asking, setAsking] = useState<"off" | "follow">();
  const refused = set.data?.keys.find((key) => key.error)?.error;
  const overrides = overrideCount(profile);
  const apply = (next: ProfileMode) => {
    if (next === "custom") onCustom();
    else onLeave();
    if (next !== "custom" || profile.mode === "off") set.mutate({ connector: connector.id, mode: next, keys: [profile.agent] }, { onSuccess: () => setAsking(undefined) });
  };
  const choose = (next: ProfileMode) => (next !== "custom" && overrides > 0 ? setAsking(next) : apply(next));
  return (
    <div className="@container px-4 py-3 sm:px-5">
      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
          <code className="min-w-0 font-mono text-sm font-medium [overflow-wrap:anywhere]" translate="no">
            {profile.agent}
          </code>
          {profile.expired ? <Tag tone="amber">{t("keys.expired")}</Tag> : null}
          {overrides > 0 ? <Tag tone="paper">{t("profile.overrides", { count: overrides })}</Tag> : null}
        </div>
        <Choice<ProfileMode>
          label={t("profile.modeLabel", { key: profile.agent, connector: connector.title })}
          disabled={set.isPending}
          value={set.isPending ? set.variables.mode : mode}
          options={MODES.map((m) => ({ value: m, text: t(`profile.mode.${m}`), hint: t(`profile.modeHint.${m}`) }))}
          onChange={choose}
        />
      </div>
      <p key={mode} className="swap-in mt-2 max-w-2xl text-sm text-pretty text-muted">
        {t(`profile.note.${mode}`, { key: profile.agent })}
      </p>
      {asking ? (
        <InlineConfirm
          label={t("profile.modeLabel", { key: profile.agent, connector: connector.title })}
          question={t("profile.dropAsk", { count: overrides, key: profile.agent, mode: t(`profile.mode.${asking}`) })}
          confirm={t("profile.dropConfirm")}
          working={t("profile.dropping")}
          busy={set.isPending}
          error={set.isError ? errorText(set.error) : undefined}
          onCancel={() => setAsking(undefined)}
          onConfirm={() => apply(asking)}
        />
      ) : null}
      <p role="alert" className="mt-2 text-sm text-amber-400 empty:hidden">
        {set.isError && !asking ? errorText(set.error) : (refused ?? "")}
      </p>
    </div>
  );
}
