import { useEffect } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { todayStr, useTitle } from "@/lib/utils";
import { PageSpinner } from "@/components/ui";

/** Opens (creating if needed) today's journal note, then jumps to the editor. */
export function DailyPage() {
  const navigate = useNavigate();
  useTitle("每日笔记");

  useEffect(() => {
    let cancelled = false;
    api
      .daily(todayStr())
      .then((note) => {
        if (cancelled) return;
        navigate(`/app/notes/${note.id}`, { replace: true });
      })
      .catch((e) => {
        if (cancelled) return;
        toast.error(e instanceof Error ? e.message : "打开日记失败");
        navigate("/app", { replace: true });
      });
    return () => {
      cancelled = true;
    };
  }, [navigate]);

  return <PageSpinner />;
}
