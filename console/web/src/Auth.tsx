import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, errorText, setCsrf } from "./api";
import { Wordmark } from "./Brand";

type AuthProps = { mode: "setup" | "login" };

/** First-visit owner creation or sign-in, in one fixed-size card. */
export function Auth({ mode }: AuthProps) {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const submit = useMutation({
    mutationFn: () => api<{ csrf: string }>("POST", mode === "setup" ? "/api/setup" : "/api/login", { username, password }),
    onSuccess: (result) => {
      setCsrf(result.csrf);
      void queryClient.invalidateQueries({ queryKey: ["session"] });
    },
  });
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    submit.mutate();
  };
  return (
    <main className="auth">
      <form className="surface auth-card rise" onSubmit={onSubmit}>
        <Wordmark />
        <h1>{mode === "setup" ? "Create the owner account" : "Sign in"}</h1>
        <p className="muted">
          {mode === "setup"
            ? "This console has one account. It lives only on this machine, in the console's volume."
            : "The owner account of this console."}
        </p>
        <label className="field-label">
          username
          <input className="field" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required />
        </label>
        <label className="field-label">
          password {mode === "setup" && <span className="muted">(10+ characters)</span>}
          <input
            className="field"
            type="password"
            autoComplete={mode === "setup" ? "new-password" : "current-password"}
            minLength={mode === "setup" ? 10 : undefined}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </label>
        <p className="error-slot" role="alert">
          {submit.isError ? errorText(submit.error) : ""}
        </p>
        <button className="button primary" disabled={submit.isPending}>
          {mode === "setup" ? "create account" : "sign in"}
        </button>
      </form>
    </main>
  );
}
