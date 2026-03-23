"use client";

import { SessionList } from "./SessionList";

interface Props {
  projectId: string;
}

export function SessionsSection({ projectId }: Props) {
  return <SessionList projectId={projectId} />;
}
