import type { Connector, CredentialMode } from "../api";
import type { Viewer } from "../people/viewer";
import { authState } from "./summary";

/** How a connector's credential card reads for the viewer: whose credential its calls use, the modes the viewer may choose between, whether they may store or sign in, their own sign-in where each person signs in, and how many people signed in, for those who manage it. */
export type CredentialsView = {
  mode: CredentialMode;
  modes: readonly CredentialMode[] | undefined;
  forms: boolean;
  signedIn: boolean | undefined;
  count: { signed: number; of: number } | undefined;
  needed: boolean;
};

/** The card's view: the mode choice and the count only once a second user exists and only for a viewer who manages connectors; where one account serves everyone only they store it, and where each person signs in every viewer signs in for themselves. */
export function credentialsViewOf(connector: Connector, viewer: Viewer): CredentialsView {
  const mode = connector.credential_mode ?? "shared";
  const manage = viewer.can("connectors.manage");
  const modes = connector.credential_modes ?? [];
  const own = mode === "per_person";
  const counted = viewer.team && manage && own && connector.signed_in_count !== undefined && connector.people_count !== undefined;
  const state = authState(connector);
  return {
    mode,
    modes: viewer.team && manage && modes.length > 1 ? modes : undefined,
    forms: own ? viewer.can("signin.own") : manage,
    signedIn: own ? connector.signed_in === true : undefined,
    count: counted ? { signed: connector.signed_in_count ?? 0, of: connector.people_count ?? 0 } : undefined,
    needed: own ? (connector.auth ?? []).length > 0 && connector.signed_in !== true : state === "key" || state === "signIn",
  };
}
