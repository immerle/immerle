/**
 * Countries offered by the admin "Concert discovery" country dropdown — an
 * ISO 3166-1 alpha-2 code sent to the server (concerts.country), matched
 * against Ticketmaster's countryCode filter, Skiddle's country filter, and
 * (France only) Eventim. Limited to countries where at least one source has
 * a real catalog (see CONCERT_PROVIDERS in concertProviders.ts) — offering a
 * country no source covers would be a dead end: enabled, configured, and
 * silently zero results forever. English names only: a technical admin
 * setting, not worth 70+ i18n keys for.
 */
export const COUNTRIES: { code: string; name: string }[] = [
  { code: 'FR', name: 'France' },
  { code: 'US', name: 'United States' },
  { code: 'GB', name: 'United Kingdom' },
  { code: 'DE', name: 'Germany' },
  { code: 'ES', name: 'Spain' },
  { code: 'IT', name: 'Italy' },
  { code: 'NL', name: 'Netherlands' },
  { code: 'BE', name: 'Belgium' },
  { code: 'IE', name: 'Ireland' },
  { code: 'CA', name: 'Canada' },
  { code: 'AU', name: 'Australia' },
  { code: 'NZ', name: 'New Zealand' },
  { code: 'SE', name: 'Sweden' },
  { code: 'NO', name: 'Norway' },
  { code: 'DK', name: 'Denmark' },
  { code: 'FI', name: 'Finland' },
  { code: 'PL', name: 'Poland' },
  { code: 'AT', name: 'Austria' },
  { code: 'CH', name: 'Switzerland' },
  { code: 'PT', name: 'Portugal' },
  { code: 'MX', name: 'Mexico' },
  { code: 'BR', name: 'Brazil' },
  { code: 'TR', name: 'Turkey' },
  { code: 'CZ', name: 'Czechia' },
  { code: 'GR', name: 'Greece' },
];
