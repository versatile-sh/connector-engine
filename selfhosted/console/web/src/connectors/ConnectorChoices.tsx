import { useState } from "react";
import type { Connector, Profile } from "../api";
import { Functions } from "./Functions";
import { ItemSections } from "./ItemSections";
import { useDefaultLens, useProfileLens, type Lens } from "./lens";
import { DefaultBar, KeyBar, ProfilePicker } from "./ProfileBar";

function Rows({ connector, lens }: { connector: Connector; lens: Lens }) {
  return (
    <>
      <p role="alert" className="-mb-6 text-sm text-amber-400 empty:hidden">
        {lens.error}
      </p>
      <Functions connector={connector} lens={lens} />
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
  const lens = useDefaultLens(connector);
  return (
    <div className="swap-in flex min-w-0 flex-col gap-10">
      <DefaultBar connector={connector} />
      <Rows connector={connector} lens={lens} />
    </div>
  );
}

function KeyView({ connector, profile }: { connector: Connector; profile: Profile }) {
  const [editing, setEditing] = useState(false);
  const mode = profile.mode === "follow" && editing ? "custom" : profile.mode;
  const lens = useProfileLens(connector, profile, mode === "custom");
  return (
    <div className="swap-in flex min-w-0 flex-col gap-10">
      <KeyBar connector={connector} profile={profile} mode={mode} onCustom={() => setEditing(true)} onLeave={() => setEditing(false)} />
      <Rows connector={connector} lens={lens} />
    </div>
  );
}

/** A connector's functions and items as one profile sees them, picked at the top: the default every key follows, or the key named by `?profile=` with its own mode and narrowings. */
export function ConnectorChoices({ connector, agent }: { connector: Connector; agent: string | null }) {
  const profile = connector.profiles.find((p) => p.agent === agent);
  return (
    <div className="flex min-w-0 flex-col gap-6">
      <ProfilePicker connector={connector} agent={profile?.agent} />
      {profile ? <KeyView key={profile.agent} connector={connector} profile={profile} /> : <DefaultView connector={connector} />}
    </div>
  );
}
