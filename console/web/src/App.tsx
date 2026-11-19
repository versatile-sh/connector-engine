import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Agents } from "./Agents";
import { api, errorText, setCsrf, type Session } from "./api";
import { Approvals, useApprovals } from "./Approvals";
import { Auth } from "./Auth";
import { Wordmark } from "./Brand";
import { Connectors } from "./Connectors";
import { ImportPage } from "./ImportPage";
import { navigate, useLocation } from "./router";

const NAV = [
  { path: "/", label: "connectors" },
  { path: "/import", label: "import" },
  { path: "/agents", label: "agents" },
  { path: "/approvals", label: "approvals" },
] as const;

const OAUTH_RESULT: Record<string, string> = {
  connected: "The account is connected.",
  refused: "The provider refused the connection.",
  unknown_state: "That connection link expired; start it again.",
  issuer_mismatch: "The answer came from another issuer than the one asked; nothing was stored.",
  failed: "The engine could not finish the connection.",
};

/** The console shell: the session decides between setup, sign-in and the pages. */
export function App() {
  const session = useQuery({
    queryKey: ["session"],
    queryFn: async () => {
      const s = await api<Session>("GET", "/api/session");
      setCsrf(s.csrf);
      return s;
    },
  });
  if (session.isPending) return null;
  if (session.isError) return <p className="error pad">{errorText(session.error)}</p>;
  if (!session.data.owner_exists) return <Auth mode="setup" />;
  if (!session.data.signed_in) return <Auth mode="login" />;
  return <Shell />;
}

function Shell() {
  const { path, query } = useLocation();
  const queryClient = useQueryClient();
  const approvals = useApprovals();
  const waiting = approvals.data?.approvals.length ?? 0;
  const logout = useMutation({
    mutationFn: () => api("POST", "/api/logout"),
    onSettled: () => {
      setCsrf(undefined);
      queryClient.clear();
    },
  });
  const oauth = query.get("oauth");
  return (
    <div className="shell">
      <header className="topbar">
        <a
          href="/"
          className="brand-link"
          onClick={(e) => {
            e.preventDefault();
            navigate("/");
          }}
        >
          <Wordmark />
          <span className="product">connectors</span>
        </a>
        <nav aria-label="console">
          {NAV.map((item) => (
            <a
              key={item.path}
              href={item.path}
              aria-current={path === item.path ? "page" : undefined}
              onClick={(e) => {
                e.preventDefault();
                navigate(item.path);
              }}
            >
              {item.label}
              {item.path === "/approvals" && waiting > 0 && <span className="badge">{waiting}</span>}
            </a>
          ))}
        </nav>
        <button className="button quiet small" onClick={() => logout.mutate()}>
          sign out
        </button>
      </header>
      <main className="content" key={path}>
        {oauth && <p className="surface notice rise">{OAUTH_RESULT[oauth] ?? oauth}</p>}
        {path === "/import" ? <ImportPage /> : path === "/agents" ? <Agents /> : path === "/approvals" ? <Approvals /> : <Connectors />}
      </main>
    </div>
  );
}
