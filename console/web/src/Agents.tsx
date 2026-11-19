import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorText, type Agent, type AgentToken } from "./api";

type AgentsData = { agents: Agent[]; mcp_url: string; max_ttl_hours: number };

const snippets = (t: AgentToken) => [
  {
    label: "Claude Code",
    text: `claude mcp add --transport http connectors ${t.mcp_url} --header "Authorization: Bearer ${t.token}"`,
  },
  {
    label: "MCP client JSON",
    text: JSON.stringify({ mcpServers: { connectors: { type: "http", url: t.mcp_url, headers: { Authorization: `Bearer ${t.token}` } } } }, null, 2),
  },
  {
    label: "cnctl",
    text: `export CN_ENDPOINT=${t.mcp_url}\nexport CN_AGENT_TOKEN=${t.token}\ncnctl tools list`,
  },
];

/** Connect an agent: mint its token and copy a ready snippet. */
export function Agents() {
  const queryClient = useQueryClient();
  const data = useQuery({ queryKey: ["agents"], queryFn: () => api<AgentsData>("GET", "/api/agents") });
  const [name, setName] = useState("claude-code");
  const [days, setDays] = useState(30);
  const mint = useMutation({
    mutationFn: () => api<AgentToken>("POST", "/api/agents", { name, ttl_hours: days * 24 }),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["agents"] }),
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    mint.mutate();
  };
  const maxDays = Math.max(1, Math.floor((data.data?.max_ttl_hours ?? 720) / 24));
  return (
    <section className="stack">
      <header className="page-head">
        <div>
          <h1>Connect your agent</h1>
          <p className="muted">
            One endpoint for every connector: <code>{data.data?.mcp_url ?? "…"}</code>
          </p>
        </div>
      </header>
      <form className="surface card rise inline-form" onSubmit={onSubmit}>
        <label className="field-label grow">
          agent name
          <input className="field" value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label className="field-label">
          valid for (days)
          <input className="field narrow" type="number" min={1} max={maxDays} value={days} onChange={(e) => setDays(Number(e.target.value))} />
        </label>
        <button className="button primary" disabled={mint.isPending}>
          mint token
        </button>
        <p className="error-slot">{mint.isError ? errorText(mint.error) : ""}</p>
      </form>
      {mint.data && <Minted token={mint.data} />}
      <div className="surface card">
        <h2>Connected agents</h2>
        {data.data && data.data.agents.length > 0 ? (
          <ul className="plain">
            {data.data.agents.map((a) => (
              <li key={a.name} className="mono-fact">
                {a.name} · token until {new Date(a.expires_at).toLocaleString()}
                {new Date(a.expires_at) < new Date() && <span className="chip warn">expired</span>}
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">None yet.</p>
        )}
        <p className="muted small">
          A token reaches every connector with the choices you set. The engine's local issuer signs with a key it makes at start, so a restart of the engine ends every token: mint again after one.
        </p>
      </div>
    </section>
  );
}

function Minted({ token }: { token: AgentToken }) {
  return (
    <div className="surface card rise">
      <p className="origin">
        token for <strong>{token.agent}</strong>, until {new Date(token.expires_at).toLocaleString()}. It is shown once.
      </p>
      {snippets(token).map((s) => (
        <Snippet key={s.label} label={s.label} text={s.text} />
      ))}
    </div>
  );
}

function Snippet({ label, text }: { label: string; text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div className="snippet">
      <div className="snippet-head">
        <span className="mono-fact">{label}</span>
        <button className="button quiet small" onClick={() => void copy()}>
          {copied ? "copied" : "copy"}
        </button>
      </div>
      <pre>{text}</pre>
    </div>
  );
}
