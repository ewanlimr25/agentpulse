interface LiveIndicatorProps {
  size?: "sm" | "md";
}

export function LiveIndicator({ size = "sm" }: LiveIndicatorProps) {
  const dotSize = size === "sm" ? "w-2 h-2" : "w-2.5 h-2.5";
  const textSize = size === "sm" ? "text-xs" : "text-sm";

  return (
    <span className={`inline-flex items-center gap-1.5 ${textSize} font-medium text-green-400`}>
      <span className={`relative flex ${dotSize}`}>
        <span
          className={`animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75`}
        />
        <span className={`relative inline-flex rounded-full ${dotSize} bg-green-500`} />
      </span>
      Live
    </span>
  );
}
