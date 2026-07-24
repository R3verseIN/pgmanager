export default function PageHeader({
  title,
  description,
}: {
  title: string;
  description?: string;
}) {
  return (
    <div>
      <h2 className="text-xl font-(--font-display) tracking-tight">{title}</h2>
      {description && (
        <p className="text-sm text-ink-muted">{description}</p>
      )}
    </div>
  );
}
