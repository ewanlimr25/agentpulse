import type { EvalConfig } from "@/lib/types";

export type EvalTier = "single" | "multi" | "custom";

/**
 * Derives the current eval tier from the project's enabled eval configs.
 * - "single": only one built-in type active (default — UI looks identical to today)
 * - "multi":  multiple built-in types active (activates toggle pills, composite badge)
 * - "custom": at least one custom eval (prompt_template present) is active
 */
export function useEvalTier(configs: EvalConfig[]): EvalTier {
  const active = configs.filter((c) => c.Enabled);
  if (active.some((c) => c.PromptTemplate !== undefined && c.PromptTemplate !== null)) {
    return "custom";
  }
  if (active.length > 1) return "multi";
  return "single";
}
