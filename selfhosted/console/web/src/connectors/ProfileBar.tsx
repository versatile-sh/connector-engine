import { useMutation } from "@tanstack/react-query";
import { errorText, type Connector, type Profile, type ProfileMode } from "../api";
import { useT } from "../i18n/I18nProvider";
import { setNewKeysOff, setProfileMode } from "../mutations";
import { navigate } from "../router";
import { Choice } from "../ui/Choice";
import { Select } from "../ui/Select";
import { Switch } from "../ui/Switch";
import { CAPTION, WELL } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { commonMode, overrideCount, profileHref } from "./profiles";

const MODES: readonly ProfileMode[] = ["off", "follow", "custom"];

const SEGMENTED_UP_TO = 4;

/** Which profile the functions and items below show, the default every key follows or one key's own, kept in the address as `?profile=`: a `Choice` for up to four entries, a `Select` past them. */
export function ProfilePicker({ connector, agent }: { connector: Connector; agent: string | undefined }) {
  const t = useT();
  if (connector.profiles.length === 0) return <p className="text-sm text-pretty text-muted">{t("profile.noKeys")}</p>;
  const label = t("profile.pickerLabel", { connector: connector.title });
  const options = [{ value: "", text: t("profile.default"), hint: t("profile.defaultHint") }, ...connector.profiles.map((p) => ({ value: p.agent, text: p.expired ? t("profile.expiredKey", { key: p.agent }) : p.agent, hint: t(`profile.modeHint.${p.mode}`) }))];
  const pick = (next: string) => navigate(profileHref(connector.id, next || undefined), { replace: true, keepScroll: true });
  return (
    <div className="@container flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
      <span className={CAPTION}>{t("profile.label")}</span>
      {options.length <= SEGMENTED_UP_TO ? (
        <Choice<string> label={label} value={agent ?? ""} options={options} onChange={pick} room="36rem" />
      ) : (
        <Select<string> label={label} value={agent ?? ""} options={options} onChange={pick} className="w-full max-w-64" />
      )}
    </div>
  );
}

/** The default's own controls: one choice that sets every key to off or the default in one request, and whether keys made from now on start off. */
export function DefaultBar({ connector }: { connector: Connector }) {
  const t = useT();
  const every = useMutation(setProfileMode);
  const newKeys = useMutation(setNewKeysOff);
  const count = (mode: ProfileMode) => connector.profiles.filter((p) => p.mode === mode).length;
  const kept = every.data?.keys.filter((key) => key.error) ?? [];
  const error = every.isError ? errorText(every.error) : newKeys.isError ? errorText(newKeys.error) : kept.length > 0 ? t("profile.everyKeyKept", { keys: kept.map((key) => key.agent).join(", ") }) : "";
  return (
    <div className={`${WELL} @container divide-y divide-line`}>
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
            onChange={(mode) => every.mutate({ connector: connector.id, mode })}
          />
        </div>
      ) : null}
      <div className="px-4 py-3 sm:px-5">
        <Switch checked={newKeys.isPending ? newKeys.variables.off : (connector.new_keys_off ?? false)} onChange={(off) => newKeys.mutate({ connector: connector.id, off })} hint={t("profile.newKeysOffHint")}>
          {t("profile.newKeysOff")}
        </Switch>
      </div>
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
  const refused = set.data?.keys.find((key) => key.error)?.error;
  const choose = (next: ProfileMode) => {
    if (next === "custom") onCustom();
    else onLeave();
    if (next !== "custom" || profile.mode === "off") set.mutate({ connector: connector.id, mode: next, keys: [profile.agent] });
  };
  const overrides = overrideCount(profile);
  return (
    <div className={`${WELL} @container px-4 py-3 sm:px-5`}>
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
      <p role="alert" className="mt-2 text-sm text-amber-400 empty:hidden">
        {set.isError ? errorText(set.error) : (refused ?? "")}
      </p>
    </div>
  );
}
