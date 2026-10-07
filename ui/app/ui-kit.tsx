import { ReactNode } from 'react';
import { ScrollView, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  Badge,
  Button,
  Card,
  Chip,
  EmptyState,
  ErrorState,
  Field,
  IconButton,
  Loading,
  SectionHeader,
} from '../src/components/ui';
import { PlayButton } from '../src/components/PlayButton';
import { Ionicon } from '../src/components/Ionicon';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Living design system / UI kit. A single screen that showcases the Spotify-
 * flavored tokens and every shared component, so the design language stays
 * consistent and reviewable. Reachable from Settings › Design.
 */
export default function UIKit() {
  const colors = useColors();
  const t = useT();
  return (
    <>
      <Stack.Screen options={{ title: t('navigation.uiKit') }} />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ paddingBottom: 48 }}>
        <View className="px-4 pt-3">
          <Text className="text-3xl font-bold tracking-tight text-foreground">{t('uiKit.title')}</Text>
          <Text className="pt-1 text-sm text-muted">{t('uiKit.subtitle')}</Text>
        </View>

        <SectionHeader title={t('uiKit.colors')} />
        <View className="flex-row flex-wrap gap-3 px-4">
          <Swatch name="background" value={colors.background} border />
          <Swatch name="surface" value={colors.surface} border />
          <Swatch name="surface-alt" value={colors.surfaceAlt} border />
          <Swatch name="foreground" value={colors.foreground} />
          <Swatch name="muted" value={colors.muted} />
          <Swatch name="primary" value={colors.primary} />
          <Swatch name="danger" value={colors.danger} />
          <Swatch name="border" value={colors.border} />
        </View>

        <SectionHeader title={t('uiKit.typography')} />
        <Card className="mx-4 gap-1">
          <Text className="text-3xl font-bold tracking-tight text-foreground">{t('uiKit.display')} · 30</Text>
          <Text className="text-2xl font-bold tracking-tight text-foreground">{t('uiKit.heading')} · 24</Text>
          <Text className="text-xl font-bold text-foreground">{t('uiKit.section')} · 20</Text>
          <Text className="text-base font-semibold text-foreground">{t('uiKit.bodyStrong')} · 16</Text>
          <Text className="text-base text-foreground">{t('uiKit.body')} · 16</Text>
          <Text className="text-sm text-muted">{t('uiKit.secondary')} · 14</Text>
          <Text className="text-xs text-muted">{t('uiKit.caption')} · 12</Text>
        </Card>

        <SectionHeader title={t('uiKit.buttons')} />
        <View className="gap-3 px-4">
          <Row>
            <Button title={t('uiKit.primary')} onPress={() => {}} />
            <Button title={t('uiKit.secondary')} variant="secondary" onPress={() => {}} />
          </Row>
          <Row>
            <Button title={t('uiKit.ghost')} variant="ghost" onPress={() => {}} />
            <Button title={t('uiKit.danger')} variant="danger" icon="trash-outline" onPress={() => {}} />
          </Row>
          <Row>
            <Button title={t('uiKit.small')} size="sm" onPress={() => {}} />
            <Button title={t('uiKit.large')} size="lg" icon="play" onPress={() => {}} />
          </Row>
          <Row>
            <Button title={t('uiKit.loading')} loading onPress={() => {}} />
            <Button title={t('uiKit.disabled')} disabled onPress={() => {}} />
          </Row>
        </View>

        <SectionHeader title={t('uiKit.playbackIcons')} />
        <Card className="mx-4 flex-row items-center justify-around">
          <PlayButton size={48} onPress={() => {}} />
          <PlayButton size={56} playing onPress={() => {}} />
          <IconButton name="heart-outline" size={26} color={colors.muted} />
          <IconButton name="shuffle" size={26} color={colors.muted} />
          <IconButton name="repeat" size={26} color={colors.primary} />
        </Card>

        <SectionHeader title={t('uiKit.chips')} />
        <View className="flex-row flex-wrap gap-2 px-4">
          <Chip label={t('components.search.albums')} active />
          <Chip label={t('components.search.artists')} />
          <Chip label={t('uiKit.genres')} />
          <Chip label={t('media.downloaded')} icon="arrow-down-circle" />
        </View>

        <SectionHeader title={t('uiKit.badges')} />
        <View className="flex-row flex-wrap gap-2 px-4">
          <Badge label={t('uiKit.default')} />
          <Badge label={t('settings.admin')} tone="primary" />
          <Badge label={t('tools.import.statusCompleted')} tone="success" />
          <Badge label={t('tools.import.statusFailed')} tone="danger" />
        </View>

        <SectionHeader title={t('uiKit.fields')} />
        <View className="gap-3 px-4">
          <Field label={t('uiKit.withIcon')} icon="search" placeholder={t('uiKit.searchPlaceholder')} />
          <Field label={t('uiKit.password')} icon="lock-closed-outline" placeholder="••••••" secureTextEntry />
        </View>

        <SectionHeader title={t('uiKit.states')} />
        <View className="gap-3 px-4">
          <Card className="h-40"><Loading label={t('uiKit.loading')} /></Card>
          <Card className="h-44"><EmptyState icon="musical-notes" title={t('uiKit.emptyTitle')} subtitle={t('uiKit.emptySubtitle')} /></Card>
          <Card className="h-44"><ErrorState message={t('uiKit.error')} onRetry={() => {}} /></Card>
        </View>

        <View className="items-center px-4 pt-8">
          <Ionicon name="musical-notes" size={20} color={colors.primary} />
          <Text className="pt-1 text-xs text-muted">Immerle · {t('uiKit.title')} v1</Text>
        </View>
      </ScrollView>
    </>
  );
}

function Row({ children }: { children: ReactNode }) {
  return <View className="flex-row gap-3">{children}</View>;
}

function Swatch({ name, value, border }: { name: string; value: string; border?: boolean }) {
  return (
    <View className="w-[22%] items-center gap-1">
      <View
        className={`h-14 w-full rounded-xl ${border ? 'border border-border' : ''}`}
        style={{ backgroundColor: value }}
      />
      <Text className="text-[10px] text-muted" numberOfLines={1}>
        {name}
      </Text>
    </View>
  );
}
