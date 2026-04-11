import type { LucideIcon } from "lucide-react";

export type NavItem = {
  label: string;
  href: (projectId: string) => string;
  icon?: LucideIcon;
  children?: NavItem[];
  badge?: string | number;
};

export type NavSection = {
  key: string;
  label: string;
  items: NavItem[];
  defaultCollapsed?: boolean;
};

import {
  LayoutDashboard,
  Play,
  Cpu,
  Layers,
  FlaskConical,
  Beaker,
  DollarSign,
  Bell,
  Users,
  Settings,
} from "lucide-react";

export const PROJECT_NAV: NavSection[] = [
  {
    key: "observe",
    label: "Observe",
    items: [
      { label: "Overview", href: (id) => `/projects/${id}/overview`, icon: LayoutDashboard },
      { label: "Runs",     href: (id) => `/projects/${id}/runs`,     icon: Play },
      { label: "Services", href: (id) => `/projects/${id}/services`, icon: Cpu },
      { label: "Sessions", href: (id) => `/projects/${id}/sessions`, icon: Layers },
    ],
  },
  {
    key: "analyze",
    label: "Analyze",
    items: [
      { label: "Evals", href: (id) => `/projects/${id}/evals`, icon: FlaskConical },
      { label: "Playground", href: (id) => `/projects/${id}/playground`, icon: Beaker },
    ],
  },
  {
    key: "operate",
    label: "Operate",
    items: [
      { label: "Budget", href: (id) => `/projects/${id}/budget`, icon: DollarSign },
      { label: "Alerts", href: (id) => `/projects/${id}/alerts`, icon: Bell },
    ],
  },
  {
    key: "configure",
    label: "Configure",
    items: [
      { label: "Users",    href: (id) => `/projects/${id}/users`,    icon: Users },
      { label: "Settings", href: (id) => `/projects/${id}/settings`, icon: Settings },
    ],
  },
];
