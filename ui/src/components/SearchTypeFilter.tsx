import { useRef, useState } from 'react';
import { Modal, Pressable, Text, View } from 'react-native';
import { SearchTypeFilter, useSearchUI } from '../search/store';
import { useAuth } from '../auth/store';
import { useT } from '../i18n/store';

const BASE_FILTERS: { key: SearchTypeFilter; labelKey: string }[] = [
  { key: 'all', labelKey: 'components.search.filterAll' },
  { key: 'artist', labelKey: 'components.search.typeArtist' },
  { key: 'album', labelKey: 'components.search.typeAlbum' },
  { key: 'song', labelKey: 'components.search.typeSong' },
  { key: 'playlist', labelKey: 'components.search.typePlaylist' },
];
const RADIO_FILTER: { key: SearchTypeFilter; labelKey: string } = { key: 'radio', labelKey: 'components.search.typeRadio' };

/**
 * Leftmost control in the search bar: a compact dropdown scoping the search
 * to one result type server-side (default: any type). The menu is anchored
 * under the button via a measured position (see AddToPlaylistButton in
 * PlayerBar.tsx for the same pattern) since the bar itself can sit anywhere
 * (centered desktop header, full-width mobile modal).
 */
export function SearchTypeFilterButton() {
  const t = useT();
  const typeFilter = useSearchUI((s) => s.typeFilter);
  const setTypeFilter = useSearchUI((s) => s.setTypeFilter);
  const canRadio = useAuth((s) => s.client?.isFeatureEnabled('internetRadio') ?? false);
  const anchorRef = useRef<View>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number; height: number } | null>(null);

  const FILTERS = canRadio ? [...BASE_FILTERS, RADIO_FILTER] : BASE_FILTERS;
  const current = FILTERS.find((f) => f.key === typeFilter) ?? FILTERS[0];
  const open = () => anchorRef.current?.measureInWindow((x, y, _w, height) => setAnchor({ x, y, height }));
  const close = () => setAnchor(null);

  return (
    <>
      <Pressable
        ref={anchorRef}
        onPress={open}
        accessibilityRole="button"
        accessibilityLabel={t(current.labelKey)}
        className="rounded-full bg-surface px-2.5 py-1 active:opacity-70"
      >
        <Text className="text-sm text-foreground">{t(current.labelKey)}</Text>
      </Pressable>

      <Modal transparent visible={!!anchor} animationType="fade" onRequestClose={close}>
        <Pressable className="flex-1" onPress={close}>
          {anchor ? (
            <View
              style={{ position: 'absolute', top: anchor.y + anchor.height + 4, left: anchor.x, width: 176 }}
              className="overflow-hidden rounded-xl border border-border bg-surface"
            >
              {FILTERS.map((f) => (
                <Pressable
                  key={f.key}
                  onPress={() => {
                    setTypeFilter(f.key);
                    close();
                  }}
                  className={`px-3 py-2.5 active:bg-surface-alt ${typeFilter === f.key ? 'bg-surface-alt' : ''}`}
                >
                  <Text className={`text-sm text-foreground ${typeFilter === f.key ? 'font-semibold' : ''}`}>
                    {t(f.labelKey)}
                  </Text>
                </Pressable>
              ))}
            </View>
          ) : null}
        </Pressable>
      </Modal>
    </>
  );
}
