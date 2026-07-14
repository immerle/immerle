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
  tint,
  height,
  children,
}: {
  url?: string;
  /** Colored gradient glow used instead of the flat gray panel when there's no
   * artwork (e.g. radio: stations rarely have cover art). */
  tint?: string;
  height: number;
  children: ReactNode;
}) {
  const colors = useColors();
  return (
    <View style={{ height }} className="overflow-hidden">
      {url ? (
        <>
          <Image
            source={{ uri: url }}
            style={StyleSheet.absoluteFill}
            blurRadius={60}
            resizeMode="cover"
          />
          {/* Darken the photo for legibility. */}
          <View style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(0,0,0,0.45)' }]} />
        </>
      ) : tint ? (
        <LinearGradient
          colors={[tint + '66', tint + '1f', 'transparent']}
          start={{ x: 0, y: 0 }}
          end={{ x: 0, y: 1 }}
          style={StyleSheet.absoluteFill}
        />
      ) : (
        <View style={[StyleSheet.absoluteFill, { backgroundColor: colors.surfaceAlt }]} />
      )}
      {/* Fade to the page background. */}
      <LinearGradient
        colors={['transparent', colors.background]}
        locations={[0.35, 1]}
        style={StyleSheet.absoluteFill}
      />
      <View style={{ flex: 1, justifyContent: 'flex-end' }}>{children}</View>
    </View>
  );
}
