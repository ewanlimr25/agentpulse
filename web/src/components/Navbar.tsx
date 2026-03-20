import Link from "next/link";

export function Navbar() {
  return (
    <header className="border-b border-[var(--border)] bg-[var(--surface)] px-6 py-3 flex items-center gap-4">
      <Link href="/" className="flex items-center gap-2">
        <span className="text-lg font-bold text-indigo-400 tracking-tight">AgentPulse</span>
      </Link>
      <span className="text-[var(--text-muted)] text-sm">AI Agent Observability</span>
    </header>
  );
}
