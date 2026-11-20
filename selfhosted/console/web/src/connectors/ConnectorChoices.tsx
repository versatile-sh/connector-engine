import { useState } from "react";
import type { Connector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { WELL } from "../ui/styles";
import { CopyPanel } from "./CopyPanel";
import { Functions } from "./Functions";
import { ItemSections } from "./ItemSections";
import { useLens } from "./lens";
import { DefaultBar, KeyBar } from "./ProfileBar";

/** A connector's functions and items as one profile sees them, a column of cards whose first holds the profile's own controls: the default every key follows, or the key named by `?profile=` with its mode and narrowings. The cards stay mounted from one profile to the next, so the filter, the list's length and the scroll stay; what one profile was doing (its bar, its edit, its copy, its picked rows) stays with that profile. */
export function ConnectorChoices({ connector, agent }: { connector: Connector; agent: string | undefined }) {
  const t = useT();
  const profile = connector.profiles.find((p) => p.agent === agent);
  const scope = profile?.agent ?? "";
  const [editing, setEditing] = useState<string>();
  const [copying, setCopying] = useState<string>();
  const mode = profile && (profile.mode === "follow" && editing === profile.agent ? "custom" : profile.mode);
  const lens = useLens(connector, profile, mode === "custom");
  const copy = connector.profiles.length > 1 ? { text: t(profile ? "bulk.copyFrom" : "bulk.copyTo"), open: () => setCopying(scope) } : undefined;
  return (
    <div className="flex min-w-0 flex-col gap-6">
      <section aria-label={t("profile.label")} className={WELL}>
        <div key={scope} className="swap-in">
          {profile && mode ? <KeyBar connector={connector} profile={profile} mode={mode} onCustom={() => setEditing(profile.agent)} onLeave={() => setEditing(undefined)} /> : <DefaultBar connector={connector} />}
        </div>
      </section>
      {copying === scope ? <CopyPanel connector={connector} target={profile} onClose={() => setCopying(undefined)} /> : null}
      <p role="alert" className="-mb-3 text-sm text-amber-400 empty:hidden">
        {lens.error}
      </p>
      <Functions connector={connector} lens={lens} scope={scope} copy={copy} />
      <ItemSections
        items={connector.items}
        variant="page"
        accessOf={(item) => lens.accessOf(item.name, item.access)}
        mostOf={(item) => lens.most(item.access)}
        pendingOf={(item) => lens.locked(item.name)}
        flagsOf={(item) => lens.flags(item.name, item.access)}
        accepts={lens.accepts}
        choose={(item, access) => lens.choose(item.name, access)}
      />
    </div>
  );
}
