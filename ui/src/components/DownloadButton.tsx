import { Song } from '../api/subsonic/types';
import { useAuth } from '../auth/store';
import { useDownloads } from '../offline/store';
import { isSupported as offlineSupported } from '../offline/fs';
import { IconButton } from './ui';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/**
 * Bulk "download for offline" button for a set of tracks (an album or playlist).
 * Hidden unless offline downloads are supported and advertised. Shows a filled
 * cloud once every track is downloaded; tapping then is a no-op (manage from the
 * Offline screen or the per-track menu).
 */
export function DownloadButton({ songs, size = 24 }: { songs: Song[]; size?: number }) {
  const t = useT();
  const colors = useColors();
  const canOffline = useAuth((s) => s.client?.isFeatureEnabled('offlineDownloads') ?? false) && offlineSupported;
  const entries = useDownloads((s) => s.entries);
  const downloadMany = useDownloads((s) => s.downloadMany);

  if (!canOffline || songs.length === 0) return null;
  const allDownloaded = songs.every((s) => entries[s.id]);

  return (
    <IconButton
      name={allDownloaded ? 'cloud-done' : 'cloud-download-outline'}
      size={size}
      color={allDownloaded ? colors.primary : colors.muted}
      onPress={() => {
        if (!allDownloaded) void downloadMany(songs);
      }}
      accessibilityLabel={allDownloaded ? t('media.downloaded') : t('media.download')}
    />
  );
}
