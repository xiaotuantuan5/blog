import { Monitor, Moon, Sun } from "lucide-react";
import { useUI, type Theme } from "@/stores/ui";
import { Button } from "./ui";

const order: Theme[] = ["light", "dark", "system"];
const labels: Record<Theme, string> = { light: "浅色", dark: "深色", system: "跟随系统" };

export function ThemeToggle({ className }: { className?: string }) {
  const { theme, setTheme } = useUI();
  const Icon = theme === "light" ? Sun : theme === "dark" ? Moon : Monitor;
  return (
    <Button
      variant="ghost"
      size="icon"
      className={className}
      title={`主题：${labels[theme]}（点击切换）`}
      aria-label="切换主题"
      onClick={() => setTheme(order[(order.indexOf(theme) + 1) % order.length])}
    >
      <Icon className="h-4 w-4" />
    </Button>
  );
}
