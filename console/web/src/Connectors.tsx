import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AccessPicker } from "./AccessPicker";
import { api, errorText, iconURL, type Access, type Connector } from "./api";
import { navigate } from "./router";

/** A listing icon proxied through the console as image bytes, or the first letter of its title. */
export function ListingIcon({ src, title }: { src?: string; title: string }) {
  const [broken, setBroken] = useState(false);
  const url = iconURL(src);
  if (!url || broken) {
    return <span className="icon icon-letter" aria-hidden="true">{title.slice(0, 1).toUpperCase()}</span>;
  }
  return <img className="icon" src={url} alt="" referrerPolicy="no-referrer" onError={() => setBroken(true)} />;
}

/** Every installed connector with its functions and credentials. */
export function Connectors() {
  const connectors = useQuery({ queryKey: ["connectors"], queryFn: () => api<{ connectors: Connector[] }>("GET", "/api/connectors") });
  if (connectors.isPending) return <p className="muted pad">loading connectors…</p>;
  if (connectors.isError) return <p className="error pad">{errorText(connectors.error)}</p>;
  const list = connectors.data.connectors;
  return (
    <section className="stack">
      <header className="page-head">
        <div>
          <h1>Connectors</h1>
          <p className="muted">What your agents can reach, function by function.</p>
        </div>
        <button className="button primary" onClick={() => navigate("/import")}>
          import an MCP
        </button>
      </header>
      {list.length === 0 ? (
        <div className="surface empty rise">
          <p>No connector yet.</p>
          <p className="muted">Paste the URL of an MCP server, or open an install link from a catalog.</p>
          <button className="button secondary" onClick={() => navigate("/import")}>
            import an MCP
          </button>
        </div>
      ) : (
        list.map((c) => <ConnectorCard key={c.id} connector={c} />)
      )}
    </section>
  );
}

function ConnectorCard({ connector }: { connector: Connector }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["connectors"] });
  const setAccess = useMutation({
    mutationFn: ({ name, access }: { name: string; access: Access }) =>
      api("PUT", `/api/connectors/${encodeURIComponent(connector.id)}/functions/${encodeURIComponent(name)}`, { access }),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: () => api("DELETE", `/api/connectors/${encodeURIComponent(connector.id)}`),
    onSuccess: refresh,
  });
  const functions = connector.functions ?? [];
  const on = functions.filter((f) => f.access !== "off").length;
  return (
    <article className="surface card rise">
      <header className="card-head">
        <ListingIcon src={connector.icon} title={connector.title} />
        <div className="card-title">
          <h2>{connector.title}</h2>
          <p className="mono-fact">
            {connector.alias}__* · {connector.listing}@{connector.version}
            {connector.remote_url ? ` · ${new URL(connector.remote_url).host}` : ""}
          </p>
        </div>
        <span className="chip">
          {on}/{functions.length} on
        </span>
        {connector.blocked && <span className="chip warn">{connector.blocked}</span>}
      </header>
      {connector.description && <p className="muted clip">{connector.description}</p>}
      <ul className="functions">
        {functions.map((f) => (
          <li key={f.name} className="function-row">
            <div className="function-name">
              <code>{f.name}</code>
              <span className="chip">{f.class || "unclassified"}</span>
              {f.state !== "active" && <span className="chip warn">{f.state}</span>}
              {f.description && <p className="muted clip">{f.description}</p>}
            </div>
            <AccessPicker
              label={`access of ${f.name}`}
              value={setAccess.isPending && setAccess.variables.name === f.name ? setAccess.variables.access : f.access}
              disabled={setAccess.isPending}
              onChange={(access) => setAccess.mutate({ name: f.name, access })}
            />
          </li>
        ))}
      </ul>
      <p className="error-slot" role="alert">
        {setAccess.isError ? errorText(setAccess.error) : remove.isError ? errorText(remove.error) : ""}
      </p>
      <footer className="card-foot">
        <button className="button quiet" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
          {open ? "hide credentials" : "credentials"}
          {connector.credentials?.length ? ` (${connector.credentials.length})` : ""}
        </button>
        <button
          className="button quiet danger"
          disabled={remove.isPending}
          onClick={() => window.confirm(`Remove ${connector.title}? Its grants go with it.`) && remove.mutate()}
        >
          remove
        </button>
      </footer>
      <div className="reveal" data-open={open}>
        <div>{open && <Credentials connector={connector} onSaved={refresh} />}</div>
      </div>
    </article>
  );
}

function Credentials({ connector, onSaved }: { connector: Connector; onSaved: () => void }) {
  const auth = connector.auth ?? [];
  const [header, setHeader] = useState("Authorization");
  const [value, setValue] = useState("");
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const saveHeader = useMutation({
    mutationFn: () => api("POST", `/api/connectors/${encodeURIComponent(connector.id)}/headers`, { headers: { [header]: value } }),
    onSuccess: () => {
      setValue("");
      onSaved();
    },
  });
  const connect = useMutation({
    mutationFn: () =>
      api<{ url: string }>("POST", `/api/connectors/${encodeURIComponent(connector.id)}/oauth`, { client_id: clientID, client_secret: clientSecret }),
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
  if (auth.length === 0) {
    return (
      <p className="muted pad-top">
        This listing declares no credential, so the engine calls it without one. Import it again with "calls need a secret header" if the server asks for a key.
      </p>
    );
  }
  return (
    <div className="credential-forms pad-top">
      {connector.credentials && connector.credentials.length > 0 && (
        <p className="mono-fact">stored: {connector.credentials.join(", ")}</p>
      )}
      {auth.some((k) => k === "custom_headers" || k === "api_key") && (
        <form className="inline-form" onSubmit={onHeader}>
          <label className="field-label">
            header
            <input className="field" value={header} onChange={(e) => setHeader(e.target.value)} required />
          </label>
          <label className="field-label grow">
            value (sealed by the engine, never shown again)
            <input className="field" type="password" autoComplete="off" value={value} onChange={(e) => setValue(e.target.value)} required />
          </label>
          <button className="button secondary" disabled={saveHeader.isPending}>
            save
          </button>
          <p className="error-slot">{saveHeader.isError ? errorText(saveHeader.error) : ""}</p>
        </form>
      )}
      {auth.includes("oauth2_user") && (
        <form className="inline-form" onSubmit={onOAuth}>
          <label className="field-label">
            client id
            <input className="field" value={clientID} onChange={(e) => setClientID(e.target.value)} placeholder="pre-registered client" />
          </label>
          <label className="field-label grow">
            client secret (optional)
            <input className="field" type="password" autoComplete="off" value={clientSecret} onChange={(e) => setClientSecret(e.target.value)} />
          </label>
          <button className="button secondary" disabled={connect.isPending}>
            connect
          </button>
          <p className="error-slot">{connect.isError ? errorText(connect.error) : ""}</p>
        </form>
      )}
    </div>
  );
}
