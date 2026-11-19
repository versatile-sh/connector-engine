import { useQuery } from "@tanstack/react-query";
import { Agents } from "./Agents";
import { api, errorText, setCsrf, type Session } from "./api";
import { Approvals } from "./Approvals";
import { Auth, waitingLinkHost } from "./Auth";
import { Mark } from "./Brand";
import { Connectors } from "./Connectors";
import { useT } from "./i18n/I18nProvider";
import { ImportPage } from "./ImportPage";
import { useLocation } from "./router";
import { Shell, type Section } from "./Shell";
import { button } from "./ui/styles";

const sectionOf = (path: string): Section => (path === "/access" || path === "/agents" ? "access" : path === "/approvals" ? "approvals" : "connectors");

function Splash({ failure, onRetry }: { failure?: string; onRetry?: () => void }) {
  const t = useT();
  return (
    <main className="grid min-h-dvh place-items-center px-6">
      <div className="flex flex-col items-center gap-4 text-center">
        <Mark className={`size-12 ${failure ? "" : "pulse"}`} />
        <p role={failure ? "alert" : "status"} className={failure ? "text-amber" : "text-muted"}>
          {failure ? `${t("app.failed")} ${failure}` : t("app.loading")}
        </p>
        {onRetry ? (
          <button type="button" className={button({ variant: "secondary", size: "sm" })} onClick={onRetry}>
            {t("common.retry")}
          </button>
        ) : null}
      </div>
    </main>
  );
}

/** The console: the session decides between setup, sign-in and the pages, and the path picks the page. */
export function App() {
  const { path, query } = useLocation();
  const session = useQuery({
    queryKey: ["session"],
    queryFn: async () => {
      const s = await api<Session>("GET", "/api/session");
      setCsrf(s.csrf);
      return s;
    },
  });
  if (session.isPending) return <Splash />;
  if (session.isError) return <Splash failure={errorText(session.error)} onRetry={() => void session.refetch()} />;
  const { owner_exists, signed_in, mcp_url } = session.data;
  if (!owner_exists) return <Auth mode="setup" endpoint={mcp_url} />;
  if (!signed_in) return <Auth mode="login" endpoint={mcp_url} waitingHost={waitingLinkHost(path, query)} />;
  const section = sectionOf(path);
  return (
    <Shell section={section} endpoint={mcp_url}>
      {path === "/import" ? <ImportPage /> : section === "access" ? <Agents /> : section === "approvals" ? <Approvals /> : <Connectors />}
    </Shell>
  );
}
