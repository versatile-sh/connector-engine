import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { errorCode, errorText, type Connector, type CredentialMode } from "../api";
import { useT } from "../i18n/I18nProvider";
import { connectOAuth, sealHeader, setCredentialMode } from "../mutations";
import { useViewer } from "../people/useViewer";
import { Choice } from "../ui/Choice";
import { Field } from "../ui/Field";
import { CAPTION, HAIRLINE, SUBTITLE, WELL, button } from "../ui/styles";
import { Tag } from "../ui/Tag";
import { credentialsViewOf, type CredentialsView } from "./credentials";
import { isActive } from "./summary";

function useConnectRefusal() {
  const t = useT();
  return (error: unknown) => (errorCode(error) === "client_registration_required" ? t("creds.manualClient") : errorText(error));
}

function WhoSignsIn({ connector, view }: { connector: Connector; view: CredentialsView }) {
  const t = useT();
  const set = useMutation(setCredentialMode);
  const modes = view.modes ?? [];
  const shown = set.isPending ? set.variables.mode : view.mode;
  return (
    <div className="mt-4 border-t border-line pt-4">
      <p className="mb-2 text-sm font-medium">{t("creds.who")}</p>
      <Choice<CredentialMode> label={t("creds.who")} value={shown} disabled={set.isPending} options={modes.map((mode) => ({ value: mode, text: t(`creds.mode.${mode}`) }))} onChange={(mode) => set.mutate({ connector: connector.id, mode })} />
      <p key={shown} className="swap-in mt-2 text-[0.8125rem] leading-snug text-pretty text-muted">
        {t(`creds.approves.${shown}`)}
      </p>
      {view.count ? <p className={`${CAPTION} mt-2`}>{t("creds.count", { signed: view.count.signed, count: view.count.of })}</p> : null}
      <p role="alert" className="mt-1 text-sm text-amber-400 empty:hidden">
        {set.isError ? errorText(set.error) : ""}
      </p>
    </div>
  );
}

/** What a connector's calls carry: the stored credentials, a secret header to seal, or a sign-in with the provider. Once a second user exists, whoever manages connectors chooses who signs in, with who approves in each mode and how many people signed in; where each person signs in, the card shows the viewer's own sign-in, and where one account serves everyone, others see it read-only. */
export function Credentials({ connector }: { connector: Connector }) {
  const t = useT();
  const viewer = useViewer();
  const view = credentialsViewOf(connector, viewer);
  const own = view.signedIn !== undefined;
  const manage = viewer.can("connectors.manage");
  const auth = connector.auth ?? [];
  const stored = connector.credentials ?? [];
  const [header, setHeader] = useState("Authorization");
  const [value, setValue] = useState("");
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const saveHeader = useMutation(sealHeader);
  const connect = useMutation(connectOAuth);
  const connectRefusal = useConnectRefusal();
  const needed = view.needed;
  const onHeader = (event: FormEvent) => {
    event.preventDefault();
    saveHeader.mutate({ connector: connector.id, header, value }, { onSuccess: () => setValue("") });
  };
  const onOAuth = (event: FormEvent) => {
    event.preventDefault();
    connect.mutate({ connector: connector.id, clientID, clientSecret }, { onSuccess: (result) => window.location.assign(result.url) });
  };
  return (
    <section aria-labelledby="creds-title" className={`${WELL} relative overflow-hidden p-5`}>
      {needed ? <span aria-hidden="true" className={HAIRLINE} /> : null}
      <h2 id="creds-title" className={SUBTITLE}>
        {t("common.credentials")}
      </h2>
      <p className="text-[0.8125rem] leading-snug text-pretty text-muted">{t(own ? "creds.perPerson" : "creds.shared")}</p>
      {own ? (
        <div className="mt-3">
          <Tag tone={view.signedIn ? "paper" : "amber"} dot>
            {t(view.signedIn ? "creds.youSignedIn" : "creds.youNot")}
          </Tag>
        </div>
      ) : stored.length > 0 ? (
        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="text-sm text-muted">{t("creds.stored")}</span>
          {stored.map((credential) => (
            <Tag key={credential} tone={isActive(credential) ? "paper" : "amber"} dot>
              {credential}
            </Tag>
          ))}
        </div>
      ) : null}
      {auth.length === 0 ? <p className="mt-2 text-sm text-pretty text-muted">{t("creds.none")}</p> : null}
      {auth.length > 0 && !view.forms ? <p className={`${CAPTION} mt-3`}>{t("creds.byAdmins")}</p> : null}
      {view.forms && auth.some((kind) => kind === "custom_headers" || kind === "api_key") ? (
        <form onSubmit={onHeader} className="mt-4">
          <Field label={t("creds.header")} mono value={header} onChange={(event) => setHeader(event.target.value)} required />
          <Field label={t("creds.value")} hint={t("creds.valueHint")} error={saveHeader.isError ? errorText(saveHeader.error) : undefined} type="password" autoComplete="off" value={value} onChange={(event) => setValue(event.target.value)} required />
          <button type="submit" disabled={saveHeader.isPending} className={`${button({ variant: "secondary", size: "sm" })} w-full`}>
            {t("common.save")}
          </button>
        </form>
      ) : null}
      {view.forms && own && !manage && auth.includes("oauth2_user") ? (
        <form onSubmit={onOAuth} className="mt-5 border-t border-line pt-4">
          <p role="alert" className="mb-2 text-sm text-amber-400 empty:hidden">
            {connect.isError ? connectRefusal(connect.error) : ""}
          </p>
          <button type="submit" disabled={connect.isPending} className={`${button({ variant: view.signedIn ? "secondary" : "primary", size: "sm" })} w-full`}>
            {t(view.signedIn ? "creds.signInAgain" : "creds.signIn", { title: connector.title })}
          </button>
        </form>
      ) : view.forms && auth.includes("oauth2_user") ? (
        <form onSubmit={onOAuth} className="mt-5 border-t border-line pt-4">
          <h3 className="mb-3 text-sm font-medium">{t("creds.oauth")}</h3>
          <Field label={t("creds.clientId")} hint={t("creds.clientIdHint")} mono value={clientID} onChange={(event) => setClientID(event.target.value)} />
          <Field label={t("creds.clientSecret")} hint={t("common.optional")} error={connect.isError ? connectRefusal(connect.error) : undefined} type="password" autoComplete="off" value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} />
          <button type="submit" disabled={connect.isPending} className={`${button({ variant: "primary", size: "sm" })} w-full`}>
            {t("creds.connect")}
          </button>
        </form>
      ) : null}
      {view.modes ? <WhoSignsIn connector={connector} view={view} /> : null}
    </section>
  );
}
