import { redirect } from "next/navigation";

// Tabs that existed before the sidebar migration.
// Keeps old ?tab=X URLs working for one release.
const LEGACY_TAB_MAP: Record<string, string> = {
  overview: "overview",
  services: "services",
  budget: "budget",
  alerts: "alerts",
  evals: "evals",
  sessions: "sessions",
  users: "users",
  settings: "settings",
};

export default async function ProjectIndexPage({
  params,
  searchParams,
}: {
  params: Promise<{ projectId: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { projectId } = await params;
  const { tab } = await searchParams;
  const target = typeof tab === "string" && LEGACY_TAB_MAP[tab]
    ? LEGACY_TAB_MAP[tab]
    : "overview";

  redirect(`/projects/${projectId}/${target}`);
}
