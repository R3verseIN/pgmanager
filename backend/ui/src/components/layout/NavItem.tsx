import { Link } from "react-router-dom";
import { cn } from "../../lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../ui/tooltip";
import type { LucideIcon } from "lucide-react";

export default function NavItem({
  to,
  icon: Icon,
  label,
  isActive,
  isCollapsed,
  className,
}: {
  to: string;
  icon: LucideIcon;
  label: string;
  isActive: boolean;
  isCollapsed: boolean;
  className?: string;
}) {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Link
            to={to}
            className={cn(
              "flex items-center rounded-md px-3 py-2 text-sm font-medium transition-all duration-200 active:scale-95",
              isCollapsed ? "justify-center" : "gap-3",
              isActive
                ? "bg-surface-2 text-foreground"
                : "text-ink-muted hover:translate-x-1 hover:bg-surface-1 hover:text-foreground",
              className
            )}
          >
            <Icon className="size-4 shrink-0" />
            {!isCollapsed && <span>{label}</span>}
          </Link>
        </TooltipTrigger>
        {isCollapsed && <TooltipContent side="right">{label}</TooltipContent>}
      </Tooltip>
    </TooltipProvider>
  );
}
