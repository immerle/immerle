import { Pressable, Text, View } from 'react-native';
import { CommentQuote } from './CommentQuote';
import { CoverArt } from './CoverArt';
import { Song } from '../api/subsonic/types';

// Podium order (2nd - 1st - 3rd), matching a physical medal podium.
const ORDER = [1, 0, 2] as const;
const COVER_SIZE: Record<number, number> = { 0: 96, 1: 74, 2: 74 };
// Pillar height per rank, 1st place tallest — the physical-podium cue the
// gold/silver/bronze colors alone don't give.
const PILLAR_HEIGHT: Record<number, number> = { 0: 56, 1: 38, 2: 26 };
const MEDAL_COLOR: Record<number, string> = { 0: '#f2c94c', 1: '#c0c0c0', 2: '#cd7f32' };

/** Top-3 podium: 2nd/1st/3rd covers on gold/silver/bronze pillars, with each entry's nostalgia note. */
export function HallOfFamePodium({ top, onPress }: { top: Song[]; onPress: (index: number) => void }) {
  const ranks = ORDER.filter((i) => top[i]);
  if (!ranks.length) return null;

  return (
    <View className="flex-row items-end gap-3">
      {ranks.map((i) => (
        <Pressable key={top[i].id} onPress={() => onPress(i)} className="items-center">
          <CoverArt coverArt={top[i].coverArt} size={COVER_SIZE[i]} rounded="rounded-xl" />
          <Text numberOfLines={1} style={{ maxWidth: COVER_SIZE[i] }} className="mt-1.5 text-center text-xs font-semibold text-white">
            {top[i].title}
          </Text>
          <Text numberOfLines={1} style={{ maxWidth: COVER_SIZE[i] }} className="text-center text-[11px] text-white/70">
            {top[i].artist}
          </Text>
          {top[i].comment ? (
            <CommentQuote
              comment={top[i].comment}
              style={{ maxWidth: COVER_SIZE[i] }}
              className="text-center text-[10px] italic text-white/60"
            />
          ) : null}
          <View
            className="mt-2 items-center justify-start rounded-t-lg pt-1"
            style={{ width: COVER_SIZE[i], height: PILLAR_HEIGHT[i], backgroundColor: MEDAL_COLOR[i] }}
          >
            <Text className="text-lg font-extrabold text-white">{i + 1}</Text>
          </View>
        </Pressable>
      ))}
    </View>
  );
}
