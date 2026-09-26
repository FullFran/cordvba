/**
 * Formats the age of a value's `at` timestamp relative to `now`, for the
 * state panel (issue #102: "each with its label ... and age"). A future
 * `at` (a prediction not yet due) is shown as "in X h" instead of a
 * negative age.
 */
export function formatAge(at: string, now: Date = new Date()): string {
  const diffMs = now.getTime() - new Date(at).getTime();

  if (diffMs < 0) {
    const hoursAhead = Math.round(-diffMs / (60 * 60 * 1000));
    return `in ${hoursAhead} h`;
  }

  const totalMinutes = Math.floor(diffMs / (60 * 1000));
  if (totalMinutes < 1) {
    return "just now";
  }
  if (totalMinutes < 60) {
    return `${totalMinutes} min ago`;
  }

  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes === 0 ? `${hours} h ago` : `${hours} h ${minutes} min ago`;
}
