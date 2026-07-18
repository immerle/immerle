/** Concert-discovery sources. A `countries` list restricts a source to the
 * markets where it actually has a usable catalog — checked live against
 * every country in COUNTRIES (see internal/ticketmaster, internal/skiddle
 * and internal/eventim's supportedCountries for the same lists and how they
 * were measured). Add a source here, nowhere else. */
export const CONCERT_PROVIDERS: { name: string; countries?: string[] }[] = [
  {
    name: 'Ticketmaster',
    countries: ['US', 'GB', 'DE', 'ES', 'IT', 'NL', 'BE', 'IE', 'CA', 'AU', 'NZ', 'SE', 'NO', 'DK', 'FI', 'PL', 'AT', 'CH', 'MX', 'BR', 'TR', 'CZ'],
  },
  { name: 'Skiddle', countries: ['GB', 'IE', 'ES', 'GR', 'PT'] },
  { name: 'Eventim', countries: ['FR'] },
];

/** Comma-separated names of the sources that apply to a given country. */
export function concertProviderNames(country: string): string {
  return concertProvidersForCountry(country)
    .map((p) => p.name)
    .join(', ');
}

/** The sources that apply to a given country. */
export function concertProvidersForCountry(country: string) {
  return CONCERT_PROVIDERS.filter((p) => !p.countries || p.countries.includes(country));
}

/** Whether a named source (e.g. "Ticketmaster") covers a given country —
 * used to hide its API key field when it won't be searched. */
export function concertProviderApplies(name: string, country: string): boolean {
  return concertProvidersForCountry(country).some((p) => p.name === name);
}
