import { useMutation, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { errorText, type Access, type Connector, type Effect, type Profile } from "../api";
import { useT } from "../i18n/I18nProvider";
import { setFunctionAccess, setItemAccess, setProfileAccess } from "../mutations";
import { profileEffectsQuery } from "../queries";
import { Tag } from "../ui/Tag";
import { button } from "../ui/styles";
import { profileAccess } from "./profiles";

/** How a connector's function and item rows read and write for one profile: the access a row shows, the most it may take, whether it is locked, the flags it adds, what a choice does, the last refusal, and whether a row may accept a changed definition. */
export type Lens = {
  accessOf(name: string, base: Access): Access;
  most(base: Access): Access | undefined;
  locked(name: string): boolean;
  flags(name: string, base: Access): ReactNode;
  choose(name: string, access: Access): void;
  error: string;
  accepts: boolean;
};

/** The default profile: each row is the connector's own choice for every key, written to the connector. */
export function useDefaultLens(connector: Connector): Lens {
  const fn = useMutation(setFunctionAccess);
  const item = useMutation(setItemAccess);
  const pending = (name: string) => [fn, item].find((m) => m.isPending && m.variables.name === name)?.variables?.access;
  const failed = [fn, item].find((m) => m.isError);
  return {
    accessOf: (name, base) => pending(name) ?? base,
    most: () => undefined,
    locked: (name) => pending(name) !== undefined,
    flags: () => null,
    choose: (name, access) => ((connector.items ?? []).some((it) => it.name === name) ? item : fn).mutate({ connector: connector.id, name, access }),
    error: failed?.error ? errorText(failed.error) : "",
    accepts: true,
  };
}

function ProfileFlags({ shown, overridden, following, effect, onReset }: { shown: Access; overridden: boolean; following: boolean; effect: Effect | undefined; onReset?: () => void }) {
  const t = useT();
  return (
    <>
      {overridden ? <Tag tone="paper">{t("profile.override")}</Tag> : following ? <Tag>{t("profile.fromDefault")}</Tag> : null}
      {effect && effect.access !== shown ? (
        <Tag tone="amber" dot title={t("profile.engineHint")}>
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

/** One key's profile: each row is what the key gets, at most the default, flagged as its own or from the default, with the engine's answer where it differs; rows change only while `editable`. */
export function useProfileLens(connector: Connector, profile: Profile, editable: boolean): Lens {
  const set = useMutation(setProfileAccess);
  const effects = useQuery(profileEffectsQuery(connector.id, profile.agent));
  const engine = new Map((effects.data ?? []).map((effect) => [effect.name, effect]));
  const refused = set.data?.items.find((item) => item.error)?.error;
  const pending = (name: string) => (set.isPending && set.variables.names.includes(name) && set.variables.access !== "default" ? set.variables.access : undefined);
  const choose = (name: string, access: Access) => set.mutate({ connector: connector.id, agent: profile.agent, names: [name], access });
  return {
    accessOf: (name, base) => pending(name) ?? profileAccess(profile, name, base).access,
    most: (base) => base,
    locked: (name) => !editable || pending(name) !== undefined,
    flags: (name, base) => {
      const { access, overridden } = profileAccess(profile, name, base);
      return <ProfileFlags shown={access} overridden={overridden} following={profile.mode !== "off"} effect={engine.get(name)} onReset={overridden && editable ? () => choose(name, base) : undefined} />;
    },
    choose,
    error: set.isError ? errorText(set.error) : effects.isError ? errorText(effects.error) : (refused ?? ""),
    accepts: false,
  };
}
