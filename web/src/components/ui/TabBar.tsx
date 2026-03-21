"use client";

interface Tab {
  key: string;
  label: string;
}

interface Props {
  tabs: Tab[];
  activeTab: string;
  onTabChange: (key: string) => void;
}

export function TabBar({ tabs, activeTab, onTabChange }: Props) {
  return (
    <div className="flex gap-1 border-b border-[var(--border)] mb-6">
      {tabs.map((tab) =>
        tab.key === activeTab ? (
          <button
            key={tab.key}
            onClick={() => onTabChange(tab.key)}
            className="border-b-2 border-indigo-500 text-[var(--text)] pb-2 px-4 text-sm font-medium -mb-px"
          >
            {tab.label}
          </button>
        ) : (
          <button
            key={tab.key}
            onClick={() => onTabChange(tab.key)}
            className="text-[var(--text-muted)] hover:text-[var(--text)] pb-2 px-4 text-sm -mb-px"
          >
            {tab.label}
          </button>
        )
      )}
    </div>
  );
}
