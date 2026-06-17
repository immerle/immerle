import { useEffect, useMemo, useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Stack, router } from 'expo-router';
import { useImports, useImportSources, useStartImport } from '../src/query/imports';
import { usePlaylists } from '../src/query/playlists';
import { Badge, Button, Card, EmptyState, Field, Loading, SectionHeader } from '../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle } from '../src/components/AdminUI';
import { PlaylistMosaic } from '../src/components/PlaylistMosaic';
import { Ionicon } from '../src/components/Ionicon';
import { ImportDTO } from '../src/api/gossignolApi';

const SOURCE_LABELS: Record<string, string> = { spotify: 'Spotify' };
const label = (s?: string) => (s ? SOURCE_LABELS[s] ?? s : '');

const STATUS_LABELS: Record<string, string> = {
  pending: 'En attente',
  queued: 'En file',
  running: 'En cours',
  completed: 'Terminé',
  failed: 'Échec',
  error: 'Erreur',
};
const statusTone = (s?: string): 'success' | 'danger' | 'primary' =>
  s === 'completed' ? 'success' : s === 'failed' || s === 'error' ? 'danger' : 'primary';

/** Import playlists from external platforms (e.g. Spotify) — gated by the
 * `playlistImport` capability. Reached from Réglages → Importer une playlist. */
export default function ImportScreen() {
  const sources = useImportSources();
  const imports = useImports();
  const playlists = usePlaylists();
  const start = useStartImport();
  const [source, setSource] = useState('');
  const [ref, setRef] = useState('');

  // Map playlistId → cover-art ids (one playlists fetch, no per-import request).
  const coverMap = useMemo(() => {
    const m = new Map<string, string[]>();
    (playlists.data ?? []).forEach((p) => p.id && m.set(p.id, p.coverArts ?? []));
    return m;
  }, [playlists.data]);

  // Default to the first configured source.
  useEffect(() => {
    if (!source && sources.data?.length) {
      const first = sources.data.find((s) => s.configured) ?? sources.data[0];
      if (first?.name) setSource(first.name);
    }
  }, [sources.data, source]);

  const configured = (sources.data ?? []).filter((s) => s.configured);
  const submit = () => {
    if (!source || !ref.trim()) return;
    start.mutate({ source, ref: ref.trim() }, { onSuccess: () => setRef('') });
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color="#1db954" title="Importer une playlist" subtitle="Depuis vos autres plateformes" />}>
        {sources.isLoading ? (
          <Loading />
        ) : !configured.length ? (
          <EmptyState
            icon="cloud-offline-outline"
            title="Aucune source configurée"
            subtitle="L'administrateur doit configurer une plateforme d'import (ex. Spotify)."
          />
        ) : (
          <Card className="gap-3">
            <CardTitle icon="cloud-download" color="#1db954" title="Nouvel import" />
            <View className="flex-row flex-wrap gap-2">
              {(sources.data ?? []).map((s) => {
                const active = source === s.name;
                return (
                  <Pressable
                    key={s.name}
                    disabled={!s.configured}
                    onPress={() => s.name && setSource(s.name)}
                    className={`flex-row items-center gap-1.5 rounded-full px-3 py-1.5 ${active ? 'bg-primary/15' : 'bg-surface-alt'} ${s.configured ? '' : 'opacity-40'}`}
                  >
                    <Text className={`text-sm ${active ? 'font-semibold text-primary' : 'text-foreground'}`}>{label(s.name)}</Text>
                    {!s.configured ? <Text className="text-[10px] text-muted">non configuré</Text> : null}
                  </Pressable>
                );
              })}
            </View>
            <Field
              label="Lien ou ID de la playlist"
              placeholder="https://open.spotify.com/playlist/…"
              autoCapitalize="none"
              autoCorrect={false}
              value={ref}
              onChangeText={setRef}
              onSubmitEditing={submit}
            />
            {start.isError ? <Text className="text-xs text-danger">Échec du lancement de l'import.</Text> : null}
            <Button title="Importer" icon="cloud-download" loading={start.isPending} disabled={!source || !ref.trim()} onPress={submit} />
          </Card>
        )}

        {imports.data?.length ? (
          <>
            <SectionHeader title="Imports récents" />
            {imports.data.map((i) => (
              <ImportRow
                key={i.id}
                imp={i}
                covers={i.playlistId ? coverMap.get(i.playlistId) ?? [] : []}
                onPress={() => i.id && router.push(`/import/${i.id}` as never)}
              />
            ))}
          </>
        ) : null}
      </AdminScroll>
    </>
  );
}

function ImportRow({ imp, covers, onPress }: { imp: ImportDTO; covers: string[]; onPress: () => void }) {
  const name = imp.sourcePlaylistName || imp.sourceRef || 'Import';
  const total = imp.total ?? 0;
  const matched = imp.matched ?? 0;
  const done = imp.status === 'completed';
  const pct = total > 0 ? Math.min((matched / total) * 100, 100) : 0;

  return (
    <Pressable onPress={onPress} className="active:opacity-80">
      <Card className="flex-row items-center gap-3">
        <PlaylistMosaic covers={covers} size={52} rounded="rounded-lg" fallbackIcon="cloud-download" />
        <View className="flex-1 gap-1">
          <View className="flex-row items-center gap-2">
            <Text className="flex-1 text-base font-semibold text-foreground" numberOfLines={1}>
              {name}
            </Text>
            <Badge label={STATUS_LABELS[imp.status ?? ''] ?? imp.status ?? ''} tone={statusTone(imp.status)} />
          </View>
          <Text className="text-xs text-muted" numberOfLines={1}>
            {label(imp.source)} · {matched}/{total} trouvés
            {imp.doubtful ? ` · ${imp.doubtful} douteux` : ''}
            {imp.missing ? ` · ${imp.missing} manquants` : ''}
            {imp.failed ? ` · ${imp.failed} échecs` : ''}
          </Text>
          {!done && total > 0 ? (
            <View className="h-1.5 w-full overflow-hidden rounded-full bg-surface-alt">
              <View className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
            </View>
          ) : null}
          {imp.error ? <Text className="text-xs text-danger">{imp.error}</Text> : null}
        </View>
        <Ionicon name="chevron-forward" size={18} color="#888" />
      </Card>
    </Pressable>
  );
}
