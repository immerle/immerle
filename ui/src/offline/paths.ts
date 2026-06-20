// Pure helpers for offline file naming. Kept free of expo-file-system so they
// stay unit-testable in the node test env.

/**
 * The on-disk basename for a track's offline copy, e.g. `<id>.<ext>`. We store
 * the basename (not an absolute file:// URI) because iOS rewrites the app
 * container path across reinstalls/updates — the URI is rebuilt at read time.
 * Both id and suffix are sanitized so the name is always a safe single segment.
 */
export function offlineFileName(id: string, suffix?: string): string {
  const ext = (suffix ?? '').toLowerCase().replace(/[^a-z0-9]/g, '') || 'audio';
  const safeId = id.replace(/[^a-zA-Z0-9_-]/g, '_');
  return `${safeId}.${ext}`;
}
