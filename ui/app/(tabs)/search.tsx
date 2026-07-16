/**
 * Search is a global overlay (`SearchOverlay`), not a real screen. Mobile
 * intercepts the tab press before it navigates here; this file just needs
 * to exist so the route resolves.
 */
export default function SearchTabPlaceholder() {
  return null;
}
