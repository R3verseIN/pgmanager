import { cn } from "../../lib/utils"

interface GradientSpotlightCardProps {
  variant?: "violet" | "magenta" | "orange" | "coral"
  children: React.ReactNode
  className?: string
}

const variantStyles = {
  violet: "bg-gradient-to-br from-[#6a4cf5] to-[#4a2cd5]",
  magenta: "bg-gradient-to-br from-[#d44df0] to-[#a42dc5]",
  orange: "bg-gradient-to-br from-[#ff7a3d] to-[#e55a1d]",
  coral: "bg-gradient-to-br from-[#ff5577] to-[#e53557]",
}

export function GradientSpotlightCard({
  variant = "violet",
  children,
  className,
}: GradientSpotlightCardProps) {
  return (
    <div
      className={cn(
        "rounded-xxl p-8 text-foreground",
        variantStyles[variant],
        className
      )}
    >
      {children}
    </div>
  )
}
