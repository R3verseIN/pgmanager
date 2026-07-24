import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "../../lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 text-sm font-medium whitespace-nowrap transition-all duration-200 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none active:scale-95 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default:
          "rounded-full bg-primary text-primary-foreground hover:bg-primary/90",
        destructive:
          "rounded-full bg-destructive text-destructive-foreground hover:bg-destructive/90",
        outline:
          "rounded-full border border-border bg-transparent text-foreground hover:bg-accent hover:text-accent-foreground",
        secondary:
          "rounded-full bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost:
          "rounded-full text-foreground hover:bg-accent hover:text-accent-foreground",
        link: "text-accent-blue underline-offset-4 hover:underline",
        translucent:
          "rounded-xxl bg-surface-2 text-foreground hover:bg-surface-2/80",
        icon:
          "rounded-full bg-secondary text-foreground hover:bg-secondary/80",
      },
      size: {
        default: "h-10 px-4 py-2",
        sm: "h-9 rounded-full px-3",
        lg: "h-11 rounded-full px-8",
        icon: "size-10 rounded-full",
        "icon-sm": "size-8 rounded-full",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button"
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = "Button"

export { Button, buttonVariants }
