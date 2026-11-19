import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorText, type Approval } from "./api";

/** Calls held for the owner, polled every few seconds. */
export function useApprovals() {
  return useQuery({
    queryKey: ["approvals"],
    queryFn: () => api<{ approvals: Approval[] }>("GET", "/api/approvals"),
    refetchInterval: 4_000,
  });
}

/** Approve or deny each held call. */
export function Approvals() {
  const queryClient = useQueryClient();
  const approvals = useApprovals();
  const decide = useMutation({
    mutationFn: ({ id, approve }: { id: string; approve: boolean }) => api("POST", `/api/approvals/${encodeURIComponent(id)}`, { approve }),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["approvals"] }),
  });
  const list = approvals.data?.approvals ?? [];
  return (
    <section className="stack">
      <header className="page-head">
        <div>
          <h1>Approvals</h1>
          <p className="muted">Calls of functions set to ask wait here. The agent polls and gets the answer once you decide.</p>
        </div>
      </header>
      {approvals.isError && <p className="error">{errorText(approvals.error)}</p>}
      {list.length === 0 ? (
        <div className="surface empty">
          <p className="muted">Nothing is waiting.</p>
        </div>
      ) : (
        list.map((a) => (
          <article key={a.id} className="surface card rise approval">
            <div>
              <h2>
                <code>{a.function}</code>
              </h2>
              <p className="mono-fact">
                {a.agent} · session {a.session} · {a.class || "unclassified"} · expires {new Date(a.expires_at).toLocaleTimeString()}
              </p>
            </div>
            <div className="row">
              <button className="button secondary" disabled={decide.isPending} onClick={() => decide.mutate({ id: a.id, approve: false })}>
                deny
              </button>
              <button className="button primary" disabled={decide.isPending} onClick={() => decide.mutate({ id: a.id, approve: true })}>
                approve
              </button>
            </div>
          </article>
        ))
      )}
      <p className="error-slot">{decide.isError ? errorText(decide.error) : ""}</p>
    </section>
  );
}
