import { useQuery } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** Lyrics for a song (synced when the track's tags carry timestamps). Cached
 * indefinitely — lyrics don't change for a given track. */
export function useLyrics(songId: string | undefined) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.lyrics(songId ?? ''),
    enabled: !!client && !!songId,
    staleTime: Infinity,
    queryFn: ({ signal }) => client!.getLyrics(songId!, signal),
  });
}
