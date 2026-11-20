import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { api, errorText, type Connector } from "../api";
import { useT } from "../i18n/I18nProvider";
import { Bubble } from "../ui/Bubble";
import { Field } from "../ui/Field";
import { SURFACE, button } from "../ui/styles";
import { isActive } from "./summary";

/** What a connector's calls carry: the stored credentials, a secret header to seal, or a sign-in with the provider. */
export function Credentials({ connector, onSaved }: { connector: Connector; onSaved(): void }) {
  const t = useT();
  const auth = connector.auth ?? [];
  const stored = connector.credentials ?? [];
  const [header, setHeader] = useState("Authorization");
  const [value, setValue] = useState("");
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const path = `/api/connectors/${encodeURIComponent(connector.id)}`;
  const saveHeader = useMutation({
    mutationFn: () => api("POST", `${path}/headers`, { headers: { [header]: value } }),
    onSuccess: () => {
      setValue("");
      onSaved();
    },
  });
  const connect = useMutation({
    mutationFn: () => api<{ url: string }>("POST", `${path}/oauth`, { client_id: clientID, client_secret: clientSecret }),
    onSuccess: (result) => window.location.assign(result.url),
  });
  const onHeader = (event: FormEvent) => {
    event.preventDefault();
    saveHeader.mutate();
  };
  const onOAuth = (event: FormEvent) => {
    event.preventDefault();
    connect.mutate();
  };
  return (
    <section aria-labelledby="creds-title" className={`${SURFACE} p-5`}>
      <h2 id="creds-title" className="text-base font-medium">
        {t("creds.title")}
      </h2>
      {stored.length > 0 ? (
        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <span className="text-sm text-muted">{t("creds.stored")}</span>
          {stored.map((credential) => (
            <Bubble key={credential} tone={isActive(credential) ? "outline" : "amber"}>
              {credential}
            </Bubble>
          ))}
        </div>
      ) : null}
      {auth.length === 0 ? <p className="mt-2 text-sm text-muted">{t("creds.none")}</p> : null}
      {auth.some((kind) => kind === "custom_headers" || kind === "api_key") ? (
        <form onSubmit={onHeader} className="mt-4">
          <Field label={t("creds.header")} mono value={header} onChange={(event) => setHeader(event.target.value)} required />
          <Field label={t("creds.value")} hint={t("creds.valueHint")} error={saveHeader.isError ? errorText(saveHeader.error) : undefined} type="password" autoComplete="off" value={value} onChange={(event) => setValue(event.target.value)} required />
          <button type="submit" disabled={saveHeader.isPending} className={`${button({ variant: "secondary", size: "sm" })} w-full`}>
            {t("common.save")}
          </button>
        </form>
      ) : null}
      {auth.includes("oauth2_user") ? (
        <form onSubmit={onOAuth} className="mt-5 border-t border-line pt-4">
          <h3 className="mb-3 text-sm font-semibold">{t("creds.oauth")}</h3>
          <Field label={t("creds.clientId")} hint={t("creds.clientIdHint")} mono value={clientID} onChange={(event) => setClientID(event.target.value)} />
          <Field label={t("creds.clientSecret")} hint={t("creds.optional")} error={connect.isError ? errorText(connect.error) : undefined} type="password" autoComplete="off" value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} />
          <button type="submit" disabled={connect.isPending} className={`${button({ variant: "accent", size: "sm" })} w-full`}>
            {t("creds.connect")}
          </button>
        </form>
      ) : null}
    </section>
  );
}
