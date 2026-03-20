interface Props {
  status: "ok" | "error" | "running" | "unset";
  size?: "sm" | "md";
}

const styles = {
  ok: "bg-green-900/40 text-green-400 border border-green-800",
  error: "bg-red-900/40 text-red-400 border border-red-800",
  running: "bg-blue-900/40 text-blue-400 border border-blue-800",
  unset: "bg-zinc-800 text-zinc-400 border border-zinc-700",
};

export function StatusBadge({ status, size = "sm" }: Props) {
  const padding = size === "sm" ? "px-2 py-0.5 text-xs" : "px-3 py-1 text-sm";
  return (
    <span className={`rounded-full font-medium ${padding} ${styles[status]}`}>
      {status}
    </span>
  );
}
