/**
 * Normalize a server URL entered by the user: add a scheme (http for local
 * hosts, https otherwise) when missing, and strip any trailing slash.
 */
export function normalizeServerUrl(url: string): string {
  let u = url.trim();
  if (!/^https?:\/\//i.test(u)) {
    const host = u.split('/')[0].split(':')[0].toLowerCase();
    const isLocal =
      host === 'localhost' ||
      host === '127.0.0.1' ||
      host === '::1' ||
      host === '[::1]' ||
      host.endsWith('.local') ||
      /^10\./.test(host) ||
      /^192\.168\./.test(host) ||
      /^172\.(1[6-9]|2\d|3[01])\./.test(host);
    u = `${isLocal ? 'http' : 'https'}://${u}`;
  }
  return u.replace(/\/+$/, '');
}
