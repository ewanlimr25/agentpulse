"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { ChevronDown, ChevronRight, ChevronLeft, Search } from "lucide-react";
import { PROJECT_NAV, type NavItem } from "@/lib/nav-config";
import { ProjectSwitcher } from "./ProjectSwitcher";

interface Props {
  projectId: string;
  onSearchOpen: () => void;
}

const sectionStorageKey = (key: string) => `nav:section:${key}`;
const SIDEBAR_STORAGE_KEY = "nav:collapsed";

export function ProjectSidebar({ projectId, onSearchOpen }: Props) {
  const pathname = usePathname();

  // Two-pass pattern for all localStorage reads — avoids hydration mismatch.
  const [mounted, setMounted] = useState(false);
  const [sectionCollapsed, setSectionCollapsed] = useState<Record<string, boolean>>({});
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [lastSection, setLastSection] = useState<string | null>(null);

  useEffect(() => {
    const sections: Record<string, boolean> = {};
    for (const section of PROJECT_NAV) {
      const stored = localStorage.getItem(sectionStorageKey(section.key));
      sections[section.key] =
        stored !== null ? stored === "true" : (section.defaultCollapsed ?? false);
    }
    setSectionCollapsed(sections);
    setSidebarCollapsed(localStorage.getItem(SIDEBAR_STORAGE_KEY) === "true");
    setMounted(true);
  }, []);

  // Auto-expand the section containing the active route on cross-section navigation.
  useEffect(() => {
    const activeSection = PROJECT_NAV.find((s) =>
      s.items.some((item) => pathname.startsWith(item.href(projectId)))
    );
    if (activeSection && activeSection.key !== lastSection) {
      setLastSection(activeSection.key);
      setSectionCollapsed((prev) => {
        const next = { ...prev, [activeSection.key]: false };
        localStorage.setItem(sectionStorageKey(activeSection.key), "false");
        return next;
      });
    }
  }, [pathname, projectId, lastSection]);

  function toggleSection(key: string) {
    setSectionCollapsed((prev) => {
      const next = { ...prev, [key]: !prev[key] };
      localStorage.setItem(sectionStorageKey(key), String(next[key]));
      return next;
    });
  }

  function toggleSidebar() {
    setSidebarCollapsed((prev) => {
      localStorage.setItem(SIDEBAR_STORAGE_KEY, String(!prev));
      return !prev;
    });
  }

  function isItemActive(item: NavItem): boolean {
    return pathname.startsWith(item.href(projectId));
  }

  const sidebar = mounted ? sidebarCollapsed : false;

  return (
    <nav
      className={`${sidebar ? "w-14" : "w-64"} shrink-0 border-r border-[var(--border)] bg-[var(--surface)] flex flex-col h-full overflow-hidden transition-[width] duration-200 ease-in-out`}
    >
      {/* Brand */}
      <div className={`flex items-center border-b border-[var(--border)] ${sidebar ? "justify-center px-0 py-4" : "px-4 py-4"}`}>
        <span className="text-lg font-bold text-indigo-400 tracking-tight">
          {sidebar ? "AP" : "AgentPulse"}
        </span>
      </div>

      {/* Project switcher */}
      <div className={`border-b border-[var(--border)] ${sidebar ? "px-1.5 py-2" : "px-3 py-3"}`}>
        <ProjectSwitcher projectId={projectId} collapsed={sidebar} />
      </div>

      {/* Nav sections */}
      <div className="flex-1 overflow-y-auto py-2">
        {PROJECT_NAV.map((section, sectionIdx) => {
          const isSectionCollapsed = mounted ? (sectionCollapsed[section.key] ?? false) : false;
          // In sidebar-collapsed mode, always show all items (sections have no meaning visually)
          const showItems = sidebar || !isSectionCollapsed;

          return (
            <div key={section.key} className={sectionIdx > 0 && sidebar ? "mt-1 pt-1 border-t border-[var(--border)]" : "mb-1"}>
              {/* Section header — hidden in sidebar-collapsed mode */}
              {!sidebar && (
                <button
                  onClick={() => toggleSection(section.key)}
                  className="w-full flex items-center justify-between px-4 py-1.5 text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider hover:text-[var(--text)] transition-colors"
                >
                  {section.label}
                  {isSectionCollapsed
                    ? <ChevronRight className="w-3 h-3" />
                    : <ChevronDown className="w-3 h-3" />
                  }
                </button>
              )}

              {showItems && (
                <ul>
                  {section.items.map((item) => {
                    const active = isItemActive(item);
                    const Icon = item.icon;

                    return (
                      <li key={item.label} className="relative group">
                        <Link
                          href={item.href(projectId)}
                          className={`flex items-center gap-2.5 py-2 text-sm rounded-md transition-colors
                            ${sidebar ? "justify-center mx-1.5 px-0" : "px-3 mx-2"}
                            ${active
                              ? "bg-indigo-600/20 text-indigo-300 font-medium"
                              : "text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-2)]"
                            }`}
                        >
                          {Icon && <Icon className="w-4 h-4 shrink-0" />}
                          {!sidebar && item.label}
                          {!sidebar && item.badge != null && (
                            <span className="ml-auto text-xs bg-indigo-600 text-white rounded-full px-1.5 py-0.5 leading-none">
                              {item.badge}
                            </span>
                          )}
                          {/* Dot indicator for badge in collapsed mode */}
                          {sidebar && item.badge != null && (
                            <span className="absolute top-1 right-1 w-1.5 h-1.5 rounded-full bg-indigo-500" />
                          )}
                        </Link>

                        {/* Hover tooltip in sidebar-collapsed mode */}
                        {sidebar && (
                          <div className="pointer-events-none absolute left-full top-1/2 -translate-y-1/2 ml-2 px-2 py-1 text-xs text-[var(--text)] bg-[var(--surface)] border border-[var(--border)] rounded-md shadow-lg whitespace-nowrap z-50 opacity-0 group-hover:opacity-100 transition-opacity">
                            {item.label}
                          </div>
                        )}

                        {/* Second tier: children (shown when parent is active, expanded sidebar only) */}
                        {!sidebar && active && item.children && item.children.length > 0 && (
                          <ul className="ml-6 mt-0.5 mb-1">
                            {item.children.map((child) => (
                              <li key={child.label}>
                                <Link
                                  href={child.href(projectId)}
                                  className={`flex items-center gap-2 px-3 py-1.5 text-xs rounded-md mx-1 transition-colors ${
                                    pathname.startsWith(child.href(projectId))
                                      ? "text-indigo-300 font-medium"
                                      : "text-[var(--text-muted)] hover:text-[var(--text)]"
                                  }`}
                                >
                                  {child.label}
                                </Link>
                              </li>
                            ))}
                          </ul>
                        )}
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          );
        })}
      </div>

      {/* Collapse toggle + Search */}
      <div className={`border-t border-[var(--border)] ${sidebar ? "px-1.5 py-2 flex flex-col gap-1.5" : "px-3 py-3 flex flex-col gap-2"}`}>
        {/* Toggle button */}
        <div className="relative group">
          <button
            onClick={toggleSidebar}
            className={`flex items-center gap-2 text-sm text-[var(--text-muted)] bg-[var(--surface-2)] border border-[var(--border)] rounded-lg hover:border-indigo-500 hover:text-[var(--text)] transition-colors
              ${sidebar ? "w-full justify-center p-2" : "w-full px-3 py-2"}`}
            aria-label={sidebar ? "Expand sidebar" : "Collapse sidebar"}
          >
            <ChevronLeft className={`w-4 h-4 shrink-0 transition-transform duration-200 ${sidebar ? "rotate-180" : ""}`} />
            {!sidebar && <span>Collapse</span>}
          </button>
          {sidebar && (
            <div className="pointer-events-none absolute left-full top-1/2 -translate-y-1/2 ml-2 px-2 py-1 text-xs text-[var(--text)] bg-[var(--surface)] border border-[var(--border)] rounded-md shadow-lg whitespace-nowrap z-50 opacity-0 group-hover:opacity-100 transition-opacity">
              Expand sidebar
            </div>
          )}
        </div>

        {/* Search */}
        <div className="relative group">
          <button
            onClick={onSearchOpen}
            className={`flex items-center gap-2 text-sm text-[var(--text-muted)] bg-[var(--surface-2)] border border-[var(--border)] rounded-lg hover:border-indigo-500 hover:text-[var(--text)] transition-colors
              ${sidebar ? "w-full justify-center p-2" : "w-full px-3 py-2"}`}
            aria-label="Search (⌘K)"
          >
            <Search className="w-4 h-4 shrink-0" />
            {!sidebar && (
              <>
                <span>Search</span>
                <kbd className="ml-auto text-xs font-mono bg-[var(--surface)] px-1.5 py-0.5 rounded border border-[var(--border)]">
                  ⌘K
                </kbd>
              </>
            )}
          </button>
          {sidebar && (
            <div className="pointer-events-none absolute left-full top-1/2 -translate-y-1/2 ml-2 px-2 py-1 text-xs text-[var(--text)] bg-[var(--surface)] border border-[var(--border)] rounded-md shadow-lg whitespace-nowrap z-50 opacity-0 group-hover:opacity-100 transition-opacity">
              Search <span className="font-mono">⌘K</span>
            </div>
          )}
        </div>
      </div>
    </nav>
  );
}
