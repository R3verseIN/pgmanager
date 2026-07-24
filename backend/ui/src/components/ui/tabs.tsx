import * as React from "react";
import { cn } from "../../lib/utils";

interface TabsContextValue {
  value: string;
  onValueChange: (value: string) => void;
}

const TabsContext = React.createContext<TabsContextValue | undefined>(undefined);

function useTabs() {
  const ctx = React.useContext(TabsContext);
  if (!ctx) throw new Error("Tabs components must be used within <Tabs>");
  return ctx;
}

interface TabsProps {
  defaultValue?: string;
  value?: string;
  onValueChange?: (value: string) => void;
  children: React.ReactNode;
  className?: string;
}

export function Tabs({ defaultValue = "", value: controlledValue, onValueChange, children, className }: TabsProps) {
  const [internalValue, setInternalValue] = React.useState(defaultValue);
  const value = controlledValue ?? internalValue;
  const handleChange = (v: string) => {
    setInternalValue(v);
    onValueChange?.(v);
  };
  return (
    <TabsContext.Provider value={{ value, onValueChange: handleChange }}>
      <div className={className}>{children}</div>
    </TabsContext.Provider>
  );
}

export function TabsList({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("inline-flex h-10 items-center justify-center gap-1 rounded-full bg-surface-1 p-1 text-ink-muted", className)}>
      {children}
    </div>
  );
}

export function TabsTrigger({ value, children, className }: { value: string; children: React.ReactNode; className?: string }) {
  const ctx = useTabs();
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center rounded-full px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-all focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50",
        ctx.value === value && "bg-surface-2 text-foreground shadow",
        ctx.value !== value && "text-ink-muted hover:text-foreground",
        className
      )}
      onClick={() => ctx.onValueChange(value)}
    >
      {children}
    </button>
  );
}

export function TabsContent({ value, children, className }: { value: string; children: React.ReactNode; className?: string }) {
  const ctx = useTabs();
  if (ctx.value !== value) return null;
  return (
    <div className={cn("mt-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none", className)}>
      {children}
    </div>
  );
}
