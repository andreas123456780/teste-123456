// Tracks whether the x-ray scan intro has already been shown to this
// browser so returning visitors land straight on the storefront.
export const INTRO_STORAGE_KEY = "nast:intro-seen";

export function shouldShowIntro(): boolean {
  try {
    return localStorage.getItem(INTRO_STORAGE_KEY) !== "1";
  } catch {
    return true;
  }
}

export function markIntroSeen(): void {
  try {
    localStorage.setItem(INTRO_STORAGE_KEY, "1");
  } catch {
    /* private mode: intro plays again next time, that's fine */
  }
}
