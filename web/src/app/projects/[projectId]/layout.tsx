import { ProjectLayoutClient } from "@/components/nav/ProjectLayoutClient";

export default async function ProjectLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;

  return (
    <ProjectLayoutClient projectId={projectId}>
      {children}
    </ProjectLayoutClient>
  );
}
