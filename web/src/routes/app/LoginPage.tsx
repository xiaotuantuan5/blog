import { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { api, setCsrfToken } from "@/lib/api";
import { keys, useMe, useSite } from "@/lib/queries";
import { useTitle } from "@/lib/utils";
import { Button, ErrorText, Field, Input, PageSpinner } from "@/components/ui";

/** Only allow redirects to paths on this site. */
function safeNext(next: string | null) {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/app";
}

export function LoginPage() {
  const [sp] = useSearchParams();
  const next = safeNext(sp.get("next"));
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: me, isPending } = useMe();
  const { data: site } = useSite();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  useTitle(`登录 - ${site?.siteTitle ?? ""}`);

  if (isPending) return <PageSpinner />;
  if (me?.loggedIn) return <Navigate to={next} replace />;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await api.login(username, password);
      setCsrfToken(res.csrfToken);
      qc.setQueryData(keys.me, res);
      navigate(next, { replace: true });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg-soft px-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-4 rounded-xl border border-border bg-bg p-6 shadow-sm">
        <div>
          <h1 className="text-xl font-semibold">登录</h1>
          <p className="mt-1 text-sm text-fg-muted">{site?.siteTitle}</p>
        </div>
        <Field label="用户名">
          <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" required />
        </Field>
        <Field label="密码">
          <Input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            autoFocus
            required
          />
        </Field>
        <ErrorText error={error} />
        <Button type="submit" variant="primary" className="w-full justify-center" loading={busy}>
          登录
        </Button>
      </form>
    </div>
  );
}
