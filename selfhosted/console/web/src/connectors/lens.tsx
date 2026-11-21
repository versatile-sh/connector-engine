import { useMutation, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { errorText, type Access, type Connector, type Effect, type Profile } from "../api";
import { useT } from "../i18n/I18nProvider";
import { setFunctionAccess, setFunctionsAccess, setItemAccess, setProfileAccess, type ItemResult } from "../mutations";
import { profileEffectsQuery } from "../queries";
import { Tag } from "../ui/Tag";
import { button } from "../ui/styles";
import { profileAccess } from "./profiles";

/** How a connector's function and item rows read and write for one profile: the access a row shows, the most it may take, whether it is locked, the flags it adds, what a choice does, the last refusal, and whether a row may accept a changed definition; `many` sets many rows in one request, with what it refused, while the rows can change. */
export type Lens = {
  accessOf(name: string, base: Access): Access;
  most(base: Access): Access | undefined;
  locked(name: string): boolean;
  flags(name: string, base: Access): ReactNode;
  choose(name: string, access: Access): void;
  error: string;
  accepts: boolean;
  many?: { set(names: readonly string[], access: Access | "default"): void; busy: boolean; refused: readonly ItemResult[]; resets: boolean; overridden(name: string): boolean };
};

function ProfileFlags({ shown, overridden, following, effect, onReset }: { shown: Access; overridden: boolean; following: boolean; effect: Effect | undefined; onReset?: () => void }) {
  const t = useT();
  return (
    <>
      {overridden ? (
        <Tag tone="paper" className="arrive">
          {t("profile.override")}
        </Tag>
      ) : following ? (
        <Tag className="arrive">{t("profile.fromDefault")}</Tag>
      ) : null}
      {effect && effect.access !== shown ? (
        <Tag tone="amber" dot title={t("profile.engineHint")} className="arrive">
          {t("profile.engine", { access: t(`access.${effect.access}`) })}
          {effect.reason ? ` · ${effect.reason}` : ""}
        </Tag>
      ) : null}
      {onReset ? (
        <button type="button" className={`${button({ variant: "quiet", size: "xs" })} arrive`} onClick={onReset}>
          {t("profile.reset")}
        </button>
      ) : null}
    </>
  );
}

/** How the rows read and write for the profile shown: with no `profile`, the default, each row the connector's own choice for every key; with one, what that key gets, at most the default, flagged as its own or from the default with the engine's answer where it differs, its rows changing only while `editable`. The writes of every profile live here, so switching profiles keeps the rows mounted; a write's pending choice, error and refusals show only on the profile it was made for. */
export function useLens(connector: Connector, profile: Profile | undefined, editable: boolean, manage = true): Lens {
  const fn = useMutation(setFunctionAccess);
  const item = useMutation(setItemAccess);
  const all = useMutation(setFunctionsAccess);
  const set = useMutation(setProfileAccess);
  const effects = useQuery({ ...profileEffectsQuery(connector.id, profile?.agent ?? ""), enabled: profile !== undefined });
  if (!profile) {
    const pending = (name: string) => [fn, item].find((m) => m.isPending && m.variables.name === name)?.variables?.access ?? (all.isPending && all.variables.names.includes(name) ? all.variables.access : undefined);
    const failed = [fn, item, all].find((m) => m.isError);
    return {
      many: manage
        ? {
            set: (names, access) => access !== "default" && all.mutate({ connector: connector.id, names, access }),
            busy: all.isPending,
            refused: all.data?.items.filter((result) => result.error) ?? [],
            resets: false,
            overridden: () => false,
          }
        : undefined,
      accessOf: (name, base) => pending(name) ?? base,
      most: () => undefined,
      locked: (name) => !manage || pending(name) !== undefined,
      flags: () => null,
      choose: (name, access) => ((connector.items ?? []).some((it) => it.name === name) ? item : fn).mutate({ connector: connector.id, name, access }),
      error: failed?.error ? errorText(failed.error) : "",
      accepts: manage,
    };
  }
  const mine = set.variables?.agent === profile.agent;
  const engine = new Map((effects.data ?? []).map((effect) => [effect.name, effect]));
  const refusedAll = mine ? (set.data?.items.filter((result) => result.error) ?? []) : [];
  const refused = refusedAll.length === 1 && set.variables?.names.length === 1 ? refusedAll[0]?.error : undefined;
  const pending = (name: string) => (mine && set.isPending && set.variables.names.includes(name) && set.variables.access !== "default" ? set.variables.access : undefined);
  const write = (names: readonly string[], access: Access | "default") => set.mutate({ connector: connector.id, agent: profile.agent, names, access });
  const choose = (name: string, access: Access) => write([name], access);
  return {
    many: editable
      ? { set: write, busy: mine && set.isPending, refused: mine && set.variables && set.variables.names.length > 1 ? refusedAll : [], resets: true, overridden: (name) => profile.mode === "custom" && name in profile.overrides }
      : undefined,
    accessOf: (name, base) => pending(name) ?? profileAccess(profile, name, base).access,
    most: (base) => base,
    locked: (name) => !editable || pending(name) !== undefined,
    flags: (name, base) => {
      const { access, overridden } = profileAccess(profile, name, base);
      return <ProfileFlags shown={access} overridden={overridden} following={profile.mode !== "off"} effect={engine.get(name)} onReset={overridden && editable ? () => choose(name, base) : undefined} />;
    },
    choose,
    error: mine && set.isError ? errorText(set.error) : effects.isError ? errorText(effects.error) : (refused ?? ""),
    accepts: false,
  };
}
