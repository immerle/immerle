import { useRef, useState } from 'react';
import { Modal, Pressable, Text, View } from 'react-native';
import { Ionicon } from './Ionicon';
import { SearchTypeFilter, useSearchUI } from '../search/store';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

const FILTERS: { key: SearchTypeFilter; labelKey: string; icon: string }[] = [
  { key: 'all', labelKey: 'components.search.filterAll', icon: 'apps-outline' },
  { key: 'artist', labelKey: 'components.search.typeArtist', icon: 'person-outline' },
  { key: 'album', labelKey: 'components.search.typeAlbum', icon: 'disc-outline' },
  { key: 'song', labelKey: 'components.search.typeSong', icon: 'musical-notes-outline' },
  { key: 'playlist', labelKey: 'components.search.typePlaylist', icon: 'list-outline' },
];

/**
 * Leftmost control in the search bar: a compact dropdown scoping the search
 * to one result type server-side (default: any type). The menu is anchored
 * under the button via a measured position (see AddToPlaylistButton in
 * PlayerBar.tsx for the same pattern) since the bar itself can sit anywhere
 * (centered desktop header, full-width mobile modal).
 */
export function SearchTypeFilterButton() {
  const t = useT();
  const colors = useColors();
  const typeFilter = useSearchUI((s) => s.typeFilter);
  const setTypeFilter = useSearchUI((s) => s.setTypeFilter);
  const anchorRef = useRef<View>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number; height: number } | null>(null);

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
        className="flex-row items-center gap-0.5 rounded-full bg-surface px-2 py-1 active:opacity-70"
      >
        <Ionicon name={current.icon} size={15} color={colors.foreground} />
        <Ionicon name="chevron-down" size={12} color={colors.muted} />
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
                  className={`flex-row items-center gap-2 px-3 py-2.5 active:bg-surface-alt ${
                    typeFilter === f.key ? 'bg-surface-alt' : ''
                  }`}
                >
                  <Ionicon name={f.icon} size={16} color={colors.foreground} />
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
