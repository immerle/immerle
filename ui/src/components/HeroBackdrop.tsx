import { ReactNode } from 'react';
import { Image, StyleSheet, View } from 'react-native';
import { LinearGradient } from 'expo-linear-gradient';
import { useColors } from '../theme/colors';

/**
 * Immersive header backdrop, Spotify-style: the artwork blown up and blurred
 * behind the content, darkened, and faded into the page background at the
 * bottom. Falls back to a flat tinted panel when there's no artwork.
 */
export function HeroBackdrop({
  url,
  height,
  children,
}: {
  url?: string;
  height: number;
  children: ReactNode;
}) {
  const colors = useColors();
  return (
    <View style={{ height }} className="overflow-hidden">
      {url ? (
        <Image
          source={{ uri: url }}
          style={StyleSheet.absoluteFill}
          blurRadius={60}
          resizeMode="cover"
        />
      ) : (
        <View style={[StyleSheet.absoluteFill, { backgroundColor: colors.surfaceAlt }]} />
      )}
      {/* Darken for legibility, then fade to the page background. */}
      <View style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(0,0,0,0.45)' }]} />
      <LinearGradient
        colors={['transparent', colors.background]}
        locations={[0.35, 1]}
        style={StyleSheet.absoluteFill}
      />
      <View style={{ flex: 1, justifyContent: 'flex-end' }}>{children}</View>
    </View>
  );
}
