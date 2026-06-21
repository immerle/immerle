import { useMemo, useState } from 'react';
import { Modal, Pressable, ScrollView, Text, TextInput, useWindowDimensions, View } from 'react-native';
import { router, usePathname } from 'expo-router';
import { usePlaylists, useCreatePlaylist } from '../query/playlists';
import { useAuth } from '../auth/store';
import { PlaylistMosaic } from './PlaylistMosaic';
import { LikedCover } from './LikedCover';
import { LocalCover } from './LocalCover';
import { Ionicon } from './Ionicon';
import { IconButton, Field, Button } from './ui';
import { useUI } from '../stores/ui';
import { Playlist } from '../api/subsonic/types';
import { useColors } from '../theme/colors';
import { WIDE_BREAKPOINT } from '../theme/layout';
import { useT } from '../i18n/store';

const EXPANDED = 248;
const COLLAPSED = 76;

type SortKey = 'recent' | 'alpha' | 'added';
const SORTS: { key: SortKey; labelKey: string }[] = [
  { key: 'recent', labelKey: 'components.sidebar.sortRecent' },
  { key: 'alpha', labelKey: 'components.sidebar.sortAlpha' },
  { key: 'added', labelKey: 'components.sidebar.sortAdded' },
];

function sortPlaylists(list: Playlist[], key: SortKey): Playlist[] {
  const copy = [...list];
  if (key === 'alpha') return copy.sort((a, b) => a.name.localeCompare(b.name));
  const field = key === 'added' ? 'created' : 'changed';
  return copy.sort((a, b) => String(b[field] ?? '').localeCompare(String(a[field] ?? '')));
}

/**
 * Desktop "Your Library" sidebar. It is the library itself — not a page-nav menu
 * — listing the user's playlists (with "Titres likés" pinned on top). Header has
 * a "Bibliothèque" title and a "+" to create a playlist. Collapses to a rail
 * that shows only the covers and the action buttons.
 */
export function LibrarySidebar() {
  const t = useT();
  const colors = useColors();
  const pathname = usePathname();
  const { width: winWidth } = useWindowDimensions();
  // Icon-only collapse is a desktop affordance; in the mobile drawer the sidebar
  // is always expanded and the collapse toggle is inert.
  const wide = winWidth >= WIDE_BREAKPOINT;
  const sidebarCollapsed = useUI((s) => s.sidebarCollapsed);
  const toggleSidebar = useUI((s) => s.toggleSidebar);
  const collapsed = wide && sidebarCollapsed;
  const toggle = wide ? toggleSidebar : () => {};
  const canDiscover = useAuth((s) => s.client?.has('publicPlaylists') ?? false);
  const canRadio = useAuth((s) => s.client?.has('internetRadio') ?? false);
  const { data: playlists } = usePlaylists();
  const [creating, setCreating] = useState(false);
  const [filter, setFilter] = useState('');
  const [filterFocused, setFilterFocused] = useState(false);
  const [sort, setSort] = useState<SortKey>('recent');
  const [sortMenu, setSortMenu] = useState(false);

  const width = collapsed ? COLLAPSED : EXPANDED;

  // Local filtering & sorting — no extra API calls; we work on the loaded list.
  const q = filter.trim().toLowerCase();
  const filtered = useMemo(
    () => sortPlaylists((playlists ?? []).filter((p) => p.name.toLowerCase().includes(q)), sort),
    [playlists, q, sort],
  );
  const likedMatches = 'titres likés'.includes(q);
  const localMatches = 'musiques locales'.includes(q);
  const radioMatches = canRadio && (t('radio.title').toLowerCase().includes(q) || 'radio'.includes(q));

  return (
    <View style={{ width }} className="border-r border-border bg-surface">
      {/* Header */}
      <View
        className={`flex-row items-center gap-2 px-3 py-3 ${collapsed ? 'justify-center' : 'justify-between'}`}
      >
        {collapsed ? (
          <IconButton name="library" size={24} color={colors.foreground} onPress={toggle} accessibilityLabel={t('components.sidebar.expandLibrary')} />
        ) : (
          <>
            <Pressable onPress={toggle} className="flex-row items-center gap-2 active:opacity-70" accessibilityLabel={t('components.sidebar.collapseLibrary')}>
              <IconButton name="library" size={22} color={colors.muted} onPress={toggle} />
              <Text className="text-base font-bold text-foreground">{t('components.sidebar.library')}</Text>
            </Pressable>
            <View className="flex-row items-center gap-1">
              {canDiscover ? (
                <IconButton name="compass-outline" size={22} color={colors.muted} onPress={() => router.push('/discover' as never)} accessibilityLabel={t('components.sidebar.publicPlaylists')} />
              ) : null}
              <IconButton name="add" size={24} color={colors.muted} onPress={() => setCreating(true)} accessibilityLabel={t('components.sidebar.newPlaylist')} />
            </View>
          </>
        )}
      </View>

      {collapsed ? (
        <View className="items-center gap-1 pb-2">
          {canDiscover ? (
            <IconButton name="compass-outline" size={22} color={colors.muted} onPress={() => router.push('/discover' as never)} accessibilityLabel={t('components.sidebar.publicPlaylists')} />
          ) : null}
          <IconButton name="add" size={22} color={colors.muted} onPress={() => setCreating(true)} accessibilityLabel={t('components.sidebar.newPlaylist')} />
        </View>
      ) : null}

      {/* Local filter + sort (no API calls) */}
      {!collapsed ? (
        <View className="flex-row items-center gap-2 px-3 pb-2">
          <View
            className={`flex-1 flex-row items-center gap-1.5 rounded-full border bg-surface-alt px-3 ${
              filterFocused ? 'border-primary' : 'border-transparent'
            }`}
          >
            <Ionicon name="search" size={15} color={colors.muted} />
            <TextInput
              value={filter}
              onChangeText={setFilter}
              onFocus={() => setFilterFocused(true)}
              onBlur={() => setFilterFocused(false)}
              placeholder={t('components.sidebar.filter')}
              placeholderTextColor={colors.muted}
              className="flex-1 py-1.5 text-sm text-foreground"
              autoCapitalize="none"
              autoCorrect={false}
            />
            {filter ? <IconButton name="close-circle" size={15} color={colors.muted} onPress={() => setFilter('')} /> : null}
          </View>
          <IconButton name="swap-vertical" size={20} color={colors.muted} onPress={() => setSortMenu((v) => !v)} accessibilityLabel={t('components.sidebar.sort')} />
        </View>
      ) : null}

      {/* Sort dropdown */}
      {sortMenu && !collapsed ? (
        <>
          <Pressable
            onPress={() => setSortMenu(false)}
            style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, zIndex: 10 }}
          />
          <View
            className="rounded-xl border border-border bg-surface py-1"
            style={{ position: 'absolute', right: 12, top: 96, width: 200, zIndex: 20 }}
          >
            <Text className="px-3 py-1 text-xs font-medium text-muted">{t('components.sidebar.sortBy')}</Text>
            {SORTS.map((s) => (
              <Pressable
                key={s.key}
                onPress={() => {
                  setSort(s.key);
                  setSortMenu(false);
                }}
                className="flex-row items-center justify-between px-3 py-2 active:bg-surface-alt"
              >
                <Text className={`text-sm ${sort === s.key ? 'text-primary' : 'text-foreground'}`}>{t(s.labelKey)}</Text>
                {sort === s.key ? <Ionicon name="checkmark" size={16} color={colors.primary} /> : null}
              </Pressable>
            ))}
          </View>
        </>
      ) : null}

      {/* Library items (filtered + sorted locally) */}
      <ScrollView contentContainerStyle={{ paddingBottom: 12 }} showsVerticalScrollIndicator>
        {likedMatches ? (
          <Row
            active={pathname === '/liked'}
            collapsed={collapsed}
            cover={<LikedCover size={48} rounded={6} />}
            title={t('components.sidebar.likedSongs')}
            subtitle={t('components.sidebar.playlist')}
            onPress={() => router.push('/liked' as never)}
          />
        ) : null}
        {localMatches ? (
          <Row
            active={pathname === '/local'}
            collapsed={collapsed}
            cover={<LocalCover size={48} rounded={6} />}
            title={t('components.sidebar.localSongs')}
            subtitle={t('components.sidebar.playlist')}
            onPress={() => router.push('/local' as never)}
          />
        ) : null}
        {radioMatches ? (
          <Row
            active={pathname === '/radios'}
            collapsed={collapsed}
            cover={
              <View className="h-12 w-12 items-center justify-center rounded-md bg-primary/15">
                <Ionicon name="radio" size={24} color={colors.primary} />
              </View>
            }
            title={t('radio.title')}
            subtitle={t('radio.tabSubtitle')}
            onPress={() => router.push('/radios' as never)}
          />
        ) : null}
        {filtered.map((p: Playlist) => (
          <Row
            key={p.id}
            active={pathname === `/playlist/${p.id}`}
            collapsed={collapsed}
            cover={<PlaylistMosaic covers={p.coverArts ?? []} size={48} rounded="rounded-md" fallbackIcon="musical-notes" />}
            title={p.name}
            subtitle={t('components.sidebar.playlist')}
            onPress={() => router.push(`/playlist/${p.id}` as never)}
          />
        ))}
        {!collapsed && filtered.length === 0 && !likedMatches && !localMatches && !radioMatches ? (
          <Text className="px-3 py-4 text-sm text-muted">{t('components.sidebar.noResults')}</Text>
        ) : null}
      </ScrollView>

      <CreateModal visible={creating} onClose={() => setCreating(false)} />
    </View>
  );
}

function Row({
  active,
  collapsed,
  cover,
  title,
  subtitle,
  onPress,
}: {
  active: boolean;
  collapsed: boolean;
  cover: React.ReactNode;
  title: string;
  subtitle: string;
  onPress: () => void;
}) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityLabel={title}
      className={`flex-row items-center gap-3 rounded-md px-3 py-1.5 ${active ? 'bg-surface-alt' : ''} active:bg-surface-alt ${
        collapsed ? 'justify-center px-0' : ''
      }`}
    >
      {cover}
      {!collapsed ? (
        <View className="flex-1">
          <Text numberOfLines={1} className={`text-sm font-medium ${active ? 'text-primary' : 'text-foreground'}`}>
            {title}
          </Text>
          <Text numberOfLines={1} className="text-xs text-muted">
            {subtitle}
          </Text>
        </View>
      ) : null}
    </Pressable>
  );
}

function CreateModal({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  const t = useT();
  const create = useCreatePlaylist();
  const [name, setName] = useState('');
  const submit = () => {
    if (!name.trim()) return;
    create.mutate(
      { name: name.trim() },
      {
        onSuccess: () => {
          setName('');
          onClose();
        },
      },
    );
  };
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <Pressable className="flex-1 items-center justify-center bg-black/50 p-6" onPress={onClose}>
        <Pressable
          className="w-full max-w-sm gap-3 rounded-2xl border border-border bg-surface p-5"
          onPress={(e) => e.stopPropagation()}
        >
          <Text className="text-lg font-bold text-foreground">{t('components.sidebar.newPlaylist')}</Text>
          <Field placeholder={t('components.sidebar.playlistNamePlaceholder')} value={name} onChangeText={setName} autoFocus onSubmitEditing={submit} />
          <View className="flex-row justify-end gap-2">
            <Button title={t('components.sidebar.cancel')} variant="secondary" onPress={onClose} />
            <Button title={t('components.sidebar.create')} icon="add" loading={create.isPending} onPress={submit} />
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}
