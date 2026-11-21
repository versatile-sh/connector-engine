import { useQuery } from "@tanstack/react-query";
import { AccessPage } from "./access/AccessPage";
import { AccountPage } from "./account/AccountPage";
import { Restored } from "./account/Restored";
import { ActivityPage } from "./activity/ActivityPage";
import { errorText } from "./api";
import { ApprovalsPage } from "./approvals/ApprovalsPage";
import { ApprovePage } from "./approve/ApprovePage";
import { Auth, waitingLinkHost } from "./Auth";
import { Mark } from "./Brand";
import { ConnectorDetail } from "./connectors/ConnectorDetail";
import { ConnectorsPage } from "./connectors/ConnectorsPage";
import { Consent } from "./access/Consent";
import { useT } from "./i18n/I18nProvider";
import { ImportLanding } from "./import/ImportLanding";
import { InvitePage } from "./invite/InvitePage";
import { NotificationsPage } from "./notifications/NotificationsPage";
import { viewerOf } from "./people/viewer";
import { sessionQuery } from "./queries";
import { SkillsPage } from "./skills/SkillsPage";
import { useLocation } from "./router";
import type { ImportSource } from "./api";
import { Shell, type Section } from "./Shell";
import { button } from "./ui/styles";

const linkedSource = (path: string, query: URLSearchParams): ImportSource | undefined => {
  if (path !== "/import") return undefined;
  const listing = query.get("listing");
  const mcp = query.get("mcp");
  return listing ? { listing } : mcp ? { mcp } : undefined;
};

const SECTIONS: Record<string, Section> = { "/access": "access", "/agents": "access", "/approvals": "approvals", "/skills": "skills", "/activity": "activity", "/notifications": "notifications", "/account": "account" };

const sectionOf = (path: string): Section => SECTIONS[path] ?? (path.startsWith("/account/") ? "account" : "connectors");

function Splash({ failure, onRetry }: { failure?: string; onRetry?: () => void }) {
  const t = useT();
  return (
    <main className="grid min-h-dvh place-items-center px-6">
      <div className="flex flex-col items-center gap-4 text-center">
        <Mark className={`size-12 ${failure ? "" : "pulse"}`} />
        <p role={failure ? "alert" : "status"} className={failure ? "text-amber-400" : "text-muted"}>
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
  const session = useQuery(sessionQuery);
  if (path === "/restored") return <Restored />;
  if (session.isPending) return <Splash />;
  if (session.isError) return <Splash failure={errorText(session.error)} onRetry={() => void session.refetch()} />;
  const { owner_exists, signed_in, mcp_url } = session.data;
  if (path === "/invite") return <InvitePage />;
  if (!owner_exists) return <Auth mode="setup" endpoint={mcp_url} />;
  if (path.startsWith("/approve/") && (signed_in || query.has("t"))) return <ApprovePage id={decodeURIComponent(path.slice("/approve/".length))} token={query.get("t")} signedIn={signed_in} />;
  if (!signed_in) return <Auth mode="login" endpoint={mcp_url} waitingHost={waitingLinkHost(path, query)} />;
  if (path === "/connect") return <Consent id={query.get("request") ?? ""} />;
  const viewer = viewerOf(session.data);
  const linked = viewer.can("connectors.manage") || viewer.can("skills.manage") ? linkedSource(path, query) : undefined;
  if (linked) return <ImportLanding key={JSON.stringify(linked)} source={linked} />;
  const section = sectionOf(path);
  const detail = path.startsWith("/connectors/") ? decodeURIComponent(path.slice("/connectors/".length)) : undefined;
  return (
    <Shell section={section}>
      {section === "access" ? (
        <AccessPage endpoint={mcp_url} />
      ) : section === "approvals" ? (
        <ApprovalsPage />
      ) : section === "skills" ? (
        <SkillsPage />
      ) : section === "activity" ? (
        <ActivityPage endpoint={mcp_url} />
      ) : section === "notifications" ? (
        <NotificationsPage />
      ) : section === "account" ? (
        <AccountPage path={path} />
      ) : detail ? (
        <ConnectorDetail id={detail} />
      ) : (
        <ConnectorsPage />
      )}
    </Shell>
  );
}
