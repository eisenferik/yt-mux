import { removeCookiePermission } from "./permissions.js";

// "enabled" also preserves permission when a save may have committed before disconnect.
export type CookieSaveResult = "enabled" | "disabled" | "unchanged";

interface CookieChange {
  ready: Promise<void>;
  granted(newlyGranted: boolean): void;
  finish(result: CookieSaveResult): Promise<void>;
}

export interface CookieState {
  begin(): CookieChange;
}

export function createCookieState(): CookieState {
  let pendingChanges = 0;
  let permissionRemovalPending = false;
  // A retained or disabled permission supersedes grant results from older changes.
  let decisionGeneration = 0;
  let previousChangesFinished = Promise.resolve();

  return {
    begin() {
      pendingChanges++;
      const ready = previousChangesFinished;
      const generationAtStart = decisionGeneration;
      let release = (): void => {};
      const done = new Promise<void>((resolve) => {
        release = resolve;
      });
      previousChangesFinished = ready.then(() => done);
      let finished = false;
      return {
        ready,
        granted: (newlyGranted) => {
          if (!finished && newlyGranted && generationAtStart === decisionGeneration) {
            permissionRemovalPending = true;
          }
        },
        finish: async (result) => {
          if (finished) {
            return;
          }
          finished = true;
          await ready;
          if (result !== "unchanged") {
            decisionGeneration++;
            permissionRemovalPending = result === "disabled";
          }
          pendingChanges--;
          try {
            if (pendingChanges === 0 && permissionRemovalPending) {
              permissionRemovalPending = false;
              await removeCookiePermission();
            }
          } finally {
            release();
          }
        },
      };
    },
  };
}
