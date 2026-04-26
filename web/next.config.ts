import type { NextConfig } from "next";

// AGENTPULSE_INDIE_BUILD=1 produces a static export under ./out, suitable for
// embedding into the Go binary via embed.FS (see backend/internal/web).
// When unset (default), the dev / SSR build is unchanged.
const indie = process.env.AGENTPULSE_INDIE_BUILD === "1";

const nextConfig: NextConfig = {
  ...(indie
    ? {
        output: "export",
        trailingSlash: true,
        images: { unoptimized: true },
      }
    : {}),
};

export default nextConfig;
