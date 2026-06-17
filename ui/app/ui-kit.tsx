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

/**
 * Living design system / UI kit. A single screen that showcases the Spotify-
 * flavored tokens and every shared component, so the design language stays
 * consistent and reviewable. Reachable from Réglages › Design.
 */
export default function UIKit() {
  const colors = useColors();
  return (
    <>
      <Stack.Screen options={{ title: 'UI Kit' }} />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ paddingBottom: 48 }}>
        <View className="px-4 pt-3">
          <Text className="text-3xl font-bold tracking-tight text-foreground">Design system</Text>
          <Text className="pt-1 text-sm text-muted">Direction Spotify · minimaliste · vert #1ED760</Text>
        </View>

        {/* Colors */}
        <SectionHeader title="Couleurs" />
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

        {/* Typography */}
        <SectionHeader title="Typographie" />
        <Card className="mx-4 gap-1">
          <Text className="text-3xl font-bold tracking-tight text-foreground">Display · 30</Text>
          <Text className="text-2xl font-bold tracking-tight text-foreground">Titre · 24</Text>
          <Text className="text-xl font-bold text-foreground">Section · 20</Text>
          <Text className="text-base font-semibold text-foreground">Corps fort · 16</Text>
          <Text className="text-base text-foreground">Corps · 16</Text>
          <Text className="text-sm text-muted">Secondaire · 14</Text>
          <Text className="text-xs text-muted">Légende · 12</Text>
        </Card>

        {/* Buttons */}
        <SectionHeader title="Boutons" />
        <View className="gap-3 px-4">
          <Row>
            <Button title="Primary" onPress={() => {}} />
            <Button title="Secondary" variant="secondary" onPress={() => {}} />
          </Row>
          <Row>
            <Button title="Ghost" variant="ghost" onPress={() => {}} />
            <Button title="Danger" variant="danger" icon="trash-outline" onPress={() => {}} />
          </Row>
          <Row>
            <Button title="Small" size="sm" onPress={() => {}} />
            <Button title="Large" size="lg" icon="play" onPress={() => {}} />
          </Row>
          <Row>
            <Button title="Loading" loading onPress={() => {}} />
            <Button title="Disabled" disabled onPress={() => {}} />
          </Row>
        </View>

        {/* Play button + icon buttons */}
        <SectionHeader title="Lecture & icônes" />
        <Card className="mx-4 flex-row items-center justify-around">
          <PlayButton size={48} onPress={() => {}} />
          <PlayButton size={56} playing onPress={() => {}} />
          <IconButton name="heart-outline" size={26} color={colors.muted} />
          <IconButton name="shuffle" size={26} color={colors.muted} />
          <IconButton name="repeat" size={26} color={colors.primary} />
        </Card>

        {/* Chips */}
        <SectionHeader title="Chips" />
        <View className="flex-row flex-wrap gap-2 px-4">
          <Chip label="Albums" active />
          <Chip label="Artistes" />
          <Chip label="Genres" />
          <Chip label="Téléchargés" icon="arrow-down-circle" />
        </View>

        {/* Badges */}
        <SectionHeader title="Badges" />
        <View className="flex-row flex-wrap gap-2 px-4">
          <Badge label="Défaut" />
          <Badge label="Admin" tone="primary" />
          <Badge label="Terminé" tone="success" />
          <Badge label="Échec" tone="danger" />
        </View>

        {/* Fields */}
        <SectionHeader title="Champs" />
        <View className="gap-3 px-4">
          <Field label="Avec icône" icon="search" placeholder="Rechercher…" />
          <Field label="Mot de passe" icon="lock-closed-outline" placeholder="••••••" secureTextEntry />
        </View>

        {/* States */}
        <SectionHeader title="États" />
        <View className="gap-3 px-4">
          <Card className="h-40"><Loading label="Chargement…" /></Card>
          <Card className="h-44"><EmptyState icon="musical-notes" title="Rien ici" subtitle="Aucun élément." /></Card>
          <Card className="h-44"><ErrorState message="Une erreur est survenue." onRetry={() => {}} /></Card>
        </View>

        <View className="items-center px-4 pt-8">
          <Ionicon name="musical-notes" size={20} color={colors.primary} />
          <Text className="pt-1 text-xs text-muted">Immerle · Design system v1</Text>
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
