import { useMemo, useState } from 'react';
import { Platform, Pressable, Text, TextInput, View } from 'react-native';
import { Stack } from 'expo-router';
import { useLogStream, LogLine } from '../../src/admin/logs';
import { Badge, EmptyState, IconButton } from '../../src/components/ui';
import { Ionicon } from '../../src/components/Ionicon';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

const LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
type Level = (typeof LEVELS)[number];

/** Live server log viewer (web only): streams structured JSON log lines over
 * SSE, with a text search and a per-severity filter over the local buffer. */
export default function AdminLogs() {
  const t = useT();
  const colors = useColors();
  const { lines, connected, supported, clear } = useLogStream();
  const [query, setQuery] = useState('');
  const [searchFocused, setSearchFocused] = useState(false);
  const [activeLevels, setActiveLevels] = useState<Set<Level>>(new Set(LEVELS));

  const levelColor: Record<Level, string> = {
    DEBUG: colors.muted,
    INFO: '#3b82f6',
    WARN: '#f59e0b',
    ERROR: colors.danger,
  };

  const counts = useMemo(() => {
    const c: Record<Level, number> = { DEBUG: 0, INFO: 0, WARN: 0, ERROR: 0 };
    for (const line of lines) {
      const lvl = (line.level ?? '').toUpperCase() as Level;
      if (lvl in c) c[lvl]++;
    }
    return c;
  }, [lines]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return lines.filter((line) => {
      const lvl = (line.level ?? '').toUpperCase() as Level;
      if (LEVELS.includes(lvl) && !activeLevels.has(lvl)) return false;
      if (!q) return true;
      return lineSearchText(line).includes(q);
    });
  }, [lines, query, activeLevels]);

  const toggleLevel = (lvl: Level) => {
    setActiveLevels((prev) => {
      const next = new Set(prev);
      if (next.has(lvl)) next.delete(lvl);
      else next.add(lvl);
      return next;
    });
  };

  // Admin surfaces are web-only — skip the SSE viewer on native.
  if (Platform.OS !== 'web') {
    return (
      <>
        <Stack.Screen options={{ title: t('admin.logs.title') }} />
        <EmptyState icon="desktop-outline" title={t('admin.logs.unsupportedTitle')} subtitle={t('admin.logs.unsupportedSubtitle')} />
      </>
    );
  }

  const filtering = query.trim() !== '' || activeLevels.size < LEVELS.length;

  return (
    <AdminScroll
      header={
        <AdminHeader
          color="#64748b"
          title={t('admin.logs.title')}
          subtitle={t('admin.logs.headerSubtitle')}
          trailing={
            <Badge
              label={t(connected ? 'admin.logs.connected' : 'admin.logs.disconnected')}
              tone={connected ? 'success' : 'danger'}
            />
          }
        />
      }
    >
      {!supported ? (
        <EmptyState icon="desktop-outline" title={t('admin.logs.unsupportedTitle')} subtitle={t('admin.logs.unsupportedSubtitle')} />
      ) : (
        <>
          <View className="flex-row flex-wrap items-center gap-2">
            <View
              className={`min-w-[220px] flex-1 flex-row items-center gap-2 rounded-full border bg-surface-alt px-4 ${
                searchFocused ? 'border-primary' : 'border-transparent'
              }`}
            >
              <Ionicon name="search" size={16} color={colors.muted} />
              <TextInput
                value={query}
                onChangeText={setQuery}
                onFocus={() => setSearchFocused(true)}
                onBlur={() => setSearchFocused(false)}
                placeholder={t('admin.logs.searchPlaceholder')}
                placeholderTextColor={colors.muted}
                className="flex-1 py-2.5 text-sm text-foreground"
                autoCapitalize="none"
                autoCorrect={false}
              />
              {query ? (
                <IconButton name="close-circle" size={16} color={colors.muted} onPress={() => setQuery('')} accessibilityLabel={t('components.topbar.clear')} />
              ) : null}
            </View>
            <IconButton name="trash-outline" size={20} color={colors.muted} onPress={clear} accessibilityLabel={t('admin.logs.clear')} />
          </View>

          <View className="flex-row flex-wrap items-center gap-2">
            {LEVELS.map((lvl) => (
              <SeverityChip
                key={lvl}
                label={lvl}
                count={counts[lvl]}
                color={levelColor[lvl]}
                active={activeLevels.has(lvl)}
                onPress={() => toggleLevel(lvl)}
              />
            ))}
            {filtering ? (
              <Text className="ml-auto text-xs text-muted">
                {t('admin.logs.showingCount', { shown: filtered.length, total: lines.length })}
              </Text>
            ) : null}
          </View>

          {!lines.length ? (
            <EmptyState icon="terminal-outline" title={t('admin.logs.emptyTitle')} subtitle={t('admin.logs.emptySubtitle')} />
          ) : !filtered.length ? (
            <EmptyState icon="filter-outline" title={t('admin.logs.noMatchesTitle')} subtitle={t('admin.logs.noMatchesSubtitle')} />
          ) : (
            <View className="overflow-hidden rounded-xl bg-surface">
              {[...filtered].reverse().map((line, i) => (
                <LogRow key={i} line={line} color={levelColor[(line.level ?? '').toUpperCase() as Level] ?? colors.muted} last={i === filtered.length - 1} />
              ))}
            </View>
          )}
        </>
      )}
    </AdminScroll>
  );
}

function lineSearchText(line: LogLine): string {
  const { time: _time, level: _level, msg, ...attrs } = line;
  return `${msg ?? ''} ${Object.entries(attrs)
    .map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join(' ')}`.toLowerCase();
}

function SeverityChip({
  label,
  count,
  color,
  active,
  onPress,
}: {
  label: string;
  count: number;
  color: string;
  active: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected: active }}
      className="flex-row items-center gap-1.5 rounded-full border px-3 py-1.5 active:opacity-80"
      style={active ? { backgroundColor: color + '22', borderColor: color + '55' } : { backgroundColor: 'transparent', borderColor: 'transparent' }}
    >
      <View className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color, opacity: active ? 1 : 0.35 }} />
      <Text className={`text-xs font-semibold ${active ? '' : 'text-muted'}`} style={active ? { color } : undefined}>
        {label}
      </Text>
      <Text className={`text-xs ${active ? '' : 'text-muted'}`} style={active ? { color, opacity: 0.7 } : undefined}>
        {count}
      </Text>
    </Pressable>
  );
}

function LogRow({ line, color, last }: { line: LogLine; color: string; last: boolean }) {
  const { time, level, msg, ...attrs } = line;
  const extra = Object.entries(attrs)
    .map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
    .join('  ');
  return (
    <View className={`flex-row gap-3 px-3 py-2 ${last ? '' : 'border-b border-border'}`}>
      <View className="w-1 self-stretch rounded-full" style={{ backgroundColor: color }} />
      <View className="w-[78px] pt-0.5">
        <Text className="font-mono text-[11px] text-muted">{time ? new Date(time).toLocaleTimeString() : ''}</Text>
      </View>
      <View className="w-14 pt-0.5">
        <Text className="text-[11px] font-bold" style={{ color }}>
          {(level ?? '').toUpperCase()}
        </Text>
      </View>
      <View className="flex-1 gap-0.5">
        <Text className="text-sm text-foreground">{msg}</Text>
        {extra ? <Text className="font-mono text-[11px] text-muted">{extra}</Text> : null}
      </View>
    </View>
  );
}
