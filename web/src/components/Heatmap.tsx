import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const WEEKS = 26; // half a year

/** GitHub-style activity heatmap: distinct notes created/edited per day. */
export function Heatmap() {
  const { data } = useQuery({
    queryKey: ["heatmap"],
    queryFn: async () => {
      const to = new Date();
      const from = new Date(to.getTime() - WEEKS * 7 * 86400_000);
      const fmt = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
      return api.heatmap(fmt(from), fmt(to));
    },
    staleTime: 5 * 60_000,
  });

  const byDay = new Map((data ?? []).map((d) => [d.day, d.count]));

  // Build columns of weeks ending this week; pad the first column so the
  // grid starts on Monday.
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const days: { day: string; count: number; future: boolean }[][] = [];
  const total = WEEKS * 7;
  const start = new Date(today.getTime() - (total - 1) * 86400_000);
  // Align the first day to Monday of its week.
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
  for (let d = new Date(start); d <= today; d.setDate(d.getDate() + 1)) {
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    const week = days[days.length - 1];
    if (!week || week.length === 7) days.push([]);
    days[days.length - 1].push({ day: key, count: byDay.get(key) ?? 0, future: false });
  }

  const level = (n: number) => (n === 0 ? 0 : n === 1 ? 1 : n <= 3 ? 2 : n <= 6 ? 3 : 4);
  const activeDays = (data ?? []).filter((d) => d.count > 0).length;

  return (
    <div className="thin-scroll overflow-x-auto">
      <div className="flex gap-[3px]">
        {days.map((week, i) => (
          <div key={i} className="flex flex-col gap-[3px]">
            {week.map((d) => (
              <span
                key={d.day}
                title={`${d.day}：${d.count} 篇笔记`}
                className={cn("heatmap-cell", `heatmap-l${level(d.count)}`)}
              />
            ))}
          </div>
        ))}
      </div>
      <div className="mt-2 flex items-center gap-1 text-xs text-fg-subtle">
        <span>近半年 {activeDays} 天有记录</span>
        <span className="ml-auto flex items-center gap-1">
          少
          {[0, 1, 2, 3, 4].map((l) => (
            <span key={l} className={cn("heatmap-cell", `heatmap-l${l}`)} />
          ))}
          多
        </span>
      </div>
    </div>
  );
}
