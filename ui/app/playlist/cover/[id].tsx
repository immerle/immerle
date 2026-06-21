import { useMemo, useState } from 'react';
import {
  Dimensions,
  Image,
  Platform,
  Pressable,
  ScrollView,
  Text,
  View,
} from 'react-native';
import Slider from '@react-native-community/slider';
import { LinearGradient } from 'expo-linear-gradient';
import * as ImagePicker from 'expo-image-picker';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useGeneratePlaylistCover } from '../../../src/query/playlists';
import { PlaylistCoverSpec } from '../../../src/api/immerle/client';
import { Button, Field, IconButton } from '../../../src/components/ui';
import { useColors } from '../../../src/theme/colors';
import { useT } from '../../../src/i18n/store';

type Mode = 'solid' | 'gradient' | 'image';
type HAlign = 'left' | 'center' | 'right';
type VAlign = 'top' | 'middle' | 'bottom';

const H_ALIGNS: HAlign[] = ['left', 'center', 'right'];
const V_ALIGNS: VAlign[] = ['top', 'middle', 'bottom'];
const MARGIN = 0.06; // matches the server's text inset

// A small fixed palette keeps the editor dependency-free (no colour-picker lib).
const PALETTE = [
  '#1db954', '#e13300', '#8d67ab', '#1e3264', '#148a08',
  '#e8115b', '#bc5900', '#509bf5', '#ff6437', '#000000', '#ffffff',
];

// Convert a degrees angle into LinearGradient start/end (matches the server's
// dx=cos, dy=sin convention closely enough for a preview).
function angleToPoints(deg: number) {
  const r = (deg * Math.PI) / 180;
  const dx = Math.cos(r) / 2;
  const dy = Math.sin(r) / 2;
  return { start: { x: 0.5 - dx, y: 0.5 - dy }, end: { x: 0.5 + dx, y: 0.5 + dy } };
}

export default function PlaylistCoverEditor() {
  const t = useT();
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const generate = useGeneratePlaylistCover();

  const P = Math.min(Dimensions.get('window').width - 32, 360);

  const [mode, setMode] = useState<Mode>('gradient');
  const [color, setColor] = useState('#1db954');
  const [color2, setColor2] = useState('#1e3264');
  const [angle, setAngle] = useState(45);
  const [bgUri, setBgUri] = useState<string>();
  const [text, setText] = useState('');
  const [textColor, setTextColor] = useState('#ffffff');
  const [fontSize, setFontSize] = useState(0.14);
  const [align, setAlign] = useState<HAlign>('center');
  const [valign, setValign] = useState<VAlign>('middle');
  const [textH, setTextH] = useState(0); // measured preview block height
  // Which colour slot the palette edits.
  const [slot, setSlot] = useState<'color' | 'color2' | 'text'>('color');

  const points = useMemo(() => angleToPoints(angle), [angle]);

  const pickBg = async () => {
    if (Platform.OS !== 'web') {
      const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
      if (!perm.granted) return;
    }
    const res = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 0.9 });
    if (res.canceled || !res.assets?.length) return;
    setBgUri(res.assets[0].uri);
    setMode('image');
  };

  const onPaletteSelect = (c: string) => {
    if (mode === 'image' || slot === 'text') setTextColor(c);
    else if (slot === 'color2') setColor2(c);
    else setColor(c);
  };

  const save = () => {
    const spec: PlaylistCoverSpec = {
      color,
      color2: mode === 'gradient' ? color2 : undefined,
      angle: mode === 'gradient' ? angle : undefined,
      text: text.trim() || undefined,
      textColor,
      fontSize,
      align,
      valign,
    };
    generate.mutate(
      { id, spec, bgUri: mode === 'image' ? bgUri : undefined },
      { onSuccess: () => router.back() },
    );
  };

  const fontPx = fontSize * P;
  const marginPx = MARGIN * P;
  const innerW = P - 2 * marginPx;
  // Position the (measured) text block by the chosen vertical anchor.
  const top =
    valign === 'top' ? marginPx : valign === 'bottom' ? P - marginPx - textH : (P - textH) / 2;

  return (
    <>
      <Stack.Screen
        options={{
          title: t('media.playlist.cover.title'),
          headerRight: () => (
            <IconButton name="checkmark" color={colors.primary} onPress={save} disabled={generate.isPending} accessibilityLabel={t('media.playlist.cover.save')} />
          ),
        }}
      />
      <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 16, gap: 16, alignItems: 'center' }}>
        {/* Preview */}
        <View style={{ width: P, height: P }} className="overflow-hidden rounded-2xl">
          {mode === 'image' && bgUri ? (
            <Image source={{ uri: bgUri }} style={{ width: P, height: P }} resizeMode="cover" />
          ) : mode === 'gradient' ? (
            <LinearGradient colors={[color, color2]} start={points.start} end={points.end} style={{ width: P, height: P }} />
          ) : (
            <View style={{ width: P, height: P, backgroundColor: color }} />
          )}
          {text.trim() ? (
            <Text
              onLayout={(e) => setTextH(e.nativeEvent.layout.height)}
              style={{
                position: 'absolute',
                left: marginPx,
                top,
                width: innerW,
                textAlign: align,
                color: textColor,
                fontSize: fontPx,
                fontWeight: '800',
                pointerEvents: 'none',
              }}
            >
              {text}
            </Text>
          ) : null}
        </View>

        <View className="w-full max-w-md gap-4">
          {/* Background mode */}
          <View className="flex-row gap-2">
            {(['solid', 'gradient', 'image'] as Mode[]).map((m) => (
              <Pressable
                key={m}
                onPress={() => (m === 'image' ? pickBg() : setMode(m))}
                className={`flex-1 items-center rounded-xl border py-2 ${mode === m ? 'border-primary bg-surface' : 'border-border'}`}
              >
                <Text className={mode === m ? 'text-primary' : 'text-muted'}>{t(`media.playlist.cover.${m}`)}</Text>
              </Pressable>
            ))}
          </View>

          {/* Colour slot selector (hidden when using an image background) */}
          {mode !== 'image' ? (
            <View className="flex-row gap-2">
              <SlotChip label={t('media.playlist.cover.bgColor')} active={slot === 'color'} swatch={color} onPress={() => setSlot('color')} />
              {mode === 'gradient' ? (
                <SlotChip label={t('media.playlist.cover.bgColor2')} active={slot === 'color2'} swatch={color2} onPress={() => setSlot('color2')} />
              ) : null}
              <SlotChip label={t('media.playlist.cover.textColor')} active={slot === 'text'} swatch={textColor} onPress={() => setSlot('text')} />
            </View>
          ) : (
            <SlotChip label={t('media.playlist.cover.textColor')} active swatch={textColor} onPress={() => setSlot('text')} />
          )}

          {/* Palette */}
          <View className="flex-row flex-wrap gap-2">
            {PALETTE.map((c) => (
              <Pressable
                key={c}
                onPress={() => onPaletteSelect(c)}
                style={{ backgroundColor: c, width: 36, height: 36 }}
                className="rounded-full border border-border"
              />
            ))}
          </View>

          {/* Gradient angle */}
          {mode === 'gradient' ? (
            <View>
              <Text className="text-sm text-muted">{t('media.playlist.cover.angle')}: {Math.round(angle)}°</Text>
              <Slider minimumValue={0} maximumValue={360} value={angle} onValueChange={setAngle} minimumTrackTintColor={colors.primary} />
            </View>
          ) : null}

          {/* Text */}
          <Field label={t('media.playlist.cover.text')} placeholder={t('media.playlist.cover.textPlaceholder')} value={text} onChangeText={setText} multiline />

          {/* Text position (9-cell grid) */}
          <View>
            <Text className="mb-1 text-sm text-muted">{t('media.playlist.cover.position')}</Text>
            <View style={{ width: 108 }} className="gap-1">
              {V_ALIGNS.map((v) => (
                <View key={v} className="flex-row gap-1">
                  {H_ALIGNS.map((h) => {
                    const active = align === h && valign === v;
                    return (
                      <Pressable
                        key={h}
                        onPress={() => {
                          setAlign(h);
                          setValign(v);
                        }}
                        style={{ width: 34, height: 34 }}
                        className={`items-center justify-center rounded-md border ${active ? 'border-primary bg-primary' : 'border-border'}`}
                      >
                        <View style={{ width: 6, height: 6 }} className={`rounded-full ${active ? 'bg-primary-foreground' : 'bg-muted'}`} />
                      </Pressable>
                    );
                  })}
                </View>
              ))}
            </View>
          </View>

          {/* Font size */}
          <View>
            <Text className="text-sm text-muted">{t('media.playlist.cover.size')}</Text>
            <Slider minimumValue={0.06} maximumValue={0.3} value={fontSize} onValueChange={setFontSize} minimumTrackTintColor={colors.primary} />
          </View>

          <Button title={t('media.playlist.cover.save')} icon="checkmark" loading={generate.isPending} onPress={save} />
        </View>
      </ScrollView>
    </>
  );
}

function SlotChip({ label, active, swatch, onPress }: { label: string; active: boolean; swatch: string; onPress: () => void }) {
  return (
    <Pressable onPress={onPress} className={`flex-row items-center gap-2 rounded-xl border px-3 py-2 ${active ? 'border-primary' : 'border-border'}`}>
      <View style={{ backgroundColor: swatch, width: 16, height: 16 }} className="rounded-full border border-border" />
      <Text className={active ? 'text-primary' : 'text-muted'}>{label}</Text>
    </Pressable>
  );
}
