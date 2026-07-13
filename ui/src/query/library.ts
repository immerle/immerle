import { useQuery } from '@tanstack/react-query';
import { useAuth } from '../auth/store';
import { qk } from './keys';

/** All library/browse hooks. Each reads the live client from the auth store. */

export function useArtist(id: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.artist(id),
    enabled: !!client && !!id,
    queryFn: () => client!.getArtist(id),
  });
}

export function useAlbum(id: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.album(id),
    enabled: !!client && !!id,
    queryFn: () => client!.getAlbum(id),
  });
}

export function useAlbumList(type: string, genre?: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.albumList(type, genre),
    enabled: !!client,
    queryFn: () => client!.getAlbumList(type, { size: 100, genre }),
  });
}

export function useSongsByGenre(genre: string) {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.songsByGenre(genre),
    enabled: !!client && !!genre,
    queryFn: () => client!.getSongsByGenre(genre, 200),
  });
}

export function useStarred() {
  const client = useAuth((s) => s.client);
  return useQuery({
    queryKey: qk.starred,
    enabled: !!client,
    queryFn: () => client!.getStarred(),
  });
}
