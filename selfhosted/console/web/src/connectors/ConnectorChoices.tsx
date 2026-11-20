import { useState } from "react";
import type { Connector, Profile } from "../api";
import { useT } from "../i18n/I18nProvider";
import { CopyPanel } from "./CopyPanel";
import { Functions } from "./Functions";
import { ItemSections } from "./ItemSections";
import { useDefaultLens, useProfileLens, type Lens } from "./lens";
import { DefaultBar, KeyBar, ProfileCard } from "./ProfileBar";

type Copy = { text: string; open(): void } | undefined;

function Rows({ connector, lens, copy }: { connector: Connector; lens: Lens; copy: Copy }) {
  return (
    <>
      <p role="alert" className="-mb-3 text-sm text-amber-400 empty:hidden">
        {lens.error}
      </p>
      <Functions connector={connector} lens={lens} copy={copy} />
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
    </>
  );
}

function DefaultView({ connector }: { connector: Connector }) {
  const t = useT();
  const lens = useDefaultLens(connector);
  const [copying, setCopying] = useState(false);
  const copy = connector.profiles.length > 1 ? { text: t("bulk.copyTo"), open: () => setCopying(true) } : undefined;
  return (
    <div className="swap-in flex min-w-0 flex-col gap-6">
      <ProfileCard connector={connector} agent={undefined}>
        <DefaultBar connector={connector} />
      </ProfileCard>
      {copying ? <CopyPanel connector={connector} onClose={() => setCopying(false)} /> : null}
      <Rows connector={connector} lens={lens} copy={copy} />
    </div>
  );
}

function KeyView({ connector, profile }: { connector: Connector; profile: Profile }) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [copying, setCopying] = useState(false);
  const mode = profile.mode === "follow" && editing ? "custom" : profile.mode;
  const lens = useProfileLens(connector, profile, mode === "custom");
  const copy = connector.profiles.length > 1 ? { text: t("bulk.copyFrom"), open: () => setCopying(true) } : undefined;
  return (
    <div className="swap-in flex min-w-0 flex-col gap-6">
      <ProfileCard connector={connector} agent={profile.agent}>
        <KeyBar connector={connector} profile={profile} mode={mode} onCustom={() => setEditing(true)} onLeave={() => setEditing(false)} />
      </ProfileCard>
      {copying ? <CopyPanel connector={connector} target={profile} onClose={() => setCopying(false)} /> : null}
      <Rows connector={connector} lens={lens} copy={copy} />
    </div>
  );
}

/** A connector's functions and items as one profile sees them, a column of cards whose first holds the picker: the default every key follows, or the key named by `?profile=` with its own mode and narrowings. */
export function ConnectorChoices({ connector, agent }: { connector: Connector; agent: string | null }) {
  const profile = connector.profiles.find((p) => p.agent === agent);
  return profile ? <KeyView key={profile.agent} connector={connector} profile={profile} /> : <DefaultView connector={connector} />;
}
