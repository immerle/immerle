/** Concert-discovery sources. Global sources apply to every country; a
 * `countries` list restricts a source to specific ones (its coverage is
 * too thin/nonexistent elsewhere) — see internal/concerts on the backend
 * for the matching provider list. Add a source here, nowhere else. */
export const CONCERT_PROVIDERS: { name: string; countries?: string[] }[] = [
  { name: 'Ticketmaster' },
  { name: 'Skiddle' },
  { name: 'Eventim', countries: ['FR'] },
];

/** Comma-separated names of the sources that apply to a given country. */
export function concertProviderNames(country: string): string {
  return CONCERT_PROVIDERS.filter((p) => !p.countries || p.countries.includes(country))
    .map((p) => p.name)
    .join(', ');
}
