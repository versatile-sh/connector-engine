import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AccessPicker } from "./AccessPicker";
import { api, errorText, type Access, type ImportSource, type Preview } from "./api";
import { ListingIcon } from "./Connectors";
import { navigate, useLocation } from "./router";

/** Import by pasted MCP URL, or confirm a deep link (?listing= or ?mcp=); nothing is installed before the click. */
export function ImportPage() {
  const { query } = useLocation();
  const linked: ImportSource | undefined = query.get("listing")
    ? { listing: query.get("listing") ?? "" }
    : query.get("mcp")
      ? { mcp: query.get("mcp") ?? "" }
      : undefined;
  const [typed, setTyped] = useState<ImportSource | undefined>();
  const source = typed ?? linked;
  return (
    <section className="stack">
      <header className="page-head">
        <div>
          <h1>{linked && !typed ? "Install from a link" : "Import an MCP"}</h1>
          <p className="muted">The console reads the server, proposes a choice per function and installs only when you confirm.</p>
        </div>
      </header>
      {!linked && <SourceForm onPreview={setTyped} />}
      {source && <Confirm key={JSON.stringify(source)} source={source} fromLink={!typed} />}
    </section>
  );
}

function SourceForm({ onPreview }: { onPreview: (s: ImportSource) => void }) {
  const [url, setUrl] = useState("");
  const [local, setLocal] = useState(false);
  const [secret, setSecret] = useState(false);
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    onPreview({ mcp: url.trim(), local, header_secret: secret });
  };
  return (
    <form className="surface card rise" onSubmit={onSubmit}>
      <label className="field-label">
        MCP server URL
        <input className="field" type="url" placeholder="https://mcp.example.com/mcp" value={url} onChange={(e) => setUrl(e.target.value)} required />
      </label>
      <div className="checks">
        <label className="check">
          <input type="checkbox" checked={local} onChange={(e) => setLocal(e.target.checked)} />
          <span>local server: allow http and loopback or private addresses for this import</span>
        </label>
        <label className="check">
          <input type="checkbox" checked={secret} onChange={(e) => setSecret(e.target.checked)} />
          <span>calls need a secret header (set it after the import)</span>
        </label>
      </div>
      <button className="button primary">read the server</button>
    </form>
  );
}

function Confirm({ source, fromLink }: { source: ImportSource; fromLink: boolean }) {
  const queryClient = useQueryClient();
  const preview = useQuery({
    queryKey: ["preview", source],
    queryFn: () => api<Preview>("POST", "/api/preview", source),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const [choices, setChoices] = useState<Record<string, Access>>({});
  const install = useMutation({
    mutationFn: () => api<{ connector: string }>("POST", "/api/import", { ...source, choices }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["connectors"] });
      navigate("/");
    },
  });
  if (preview.isPending) return <div className="surface card rise muted">reading {source.listing ?? source.mcp}…</div>;
  if (preview.isError) return <div className="surface card rise error">{errorText(preview.error)}</div>;
  const p = preview.data;
  return (
    <article className="surface card rise">
      {fromLink && (
        <p className="origin">
          an install link asks to add a connector from <strong>{p.host}</strong>
        </p>
      )}
      <header className="card-head">
        <ListingIcon src={p.icon} title={p.title} />
        <div className="card-title">
          <h2>{p.title}</h2>
          <p className="mono-fact">
            {p.name}@{p.version}
            {p.publisher ? ` · by ${p.publisher}` : ""}
            {p.protocol ? ` · MCP ${p.protocol}` : ""}
          </p>
        </div>
      </header>
      {p.description && <p className="muted">{p.description}</p>}
      <dl className="facts">
        <dt>from</dt>
        <dd>{p.url}</dd>
        {p.remote_url && p.remote_url !== p.url && (
          <>
            <dt>server</dt>
            <dd>{p.remote_url}</dd>
          </>
        )}
        {p.auth && p.auth.length > 0 && (
          <>
            <dt>credentials</dt>
            <dd>{p.auth.join(", ")}</dd>
          </>
        )}
      </dl>
      {p.warning && <p className="warn-text">{p.warning}</p>}
      <ul className="functions">
        {p.tools.map((t) => (
          <li key={t.name} className="function-row">
            <div className="function-name">
              <code>{t.name}</code>
              <span className="chip">{t.read_only ? "read-only hint" : t.destructive ? "may change or delete" : "changes data"}</span>
              {t.description && <p className="muted clip">{t.description}</p>}
            </div>
            <AccessPicker
              label={`access of ${t.name}`}
              value={choices[t.name] ?? t.proposed}
              onChange={(access) => setChoices((prev) => ({ ...prev, [t.name]: access }))}
            />
          </li>
        ))}
      </ul>
      <p className="error-slot" role="alert">
        {install.isError ? errorText(install.error) : ""}
      </p>
      <footer className="card-foot">
        <button className="button quiet" onClick={() => navigate("/")}>
          cancel
        </button>
        <button className="button primary" disabled={install.isPending} onClick={() => install.mutate()}>
          {install.isPending ? "installing…" : `install ${p.tools.length} functions`}
        </button>
      </footer>
    </article>
  );
}
