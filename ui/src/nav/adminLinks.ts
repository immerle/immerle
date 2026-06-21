export interface AdminLink {
  href: string;
  icon: string;
  titleKey: string;
  subtitleKey: string;
  color: string;
  /** When set, hidden unless the instance advertises this. */
  requires?: 'dynamicProviders' | 'runtimeSettings' | 'libraryAdmin' | 'internetRadio' | 'federation';
}

/** Admin management destinations, shared by the admin home grid and the desktop admin sidebar. */
export const ADMIN_LINKS: AdminLink[] = [
  { href: '/admin/users', icon: 'people', titleKey: 'home.admin.link.users.title', subtitleKey: 'home.admin.link.users.subtitle', color: '#3b82f6' },
  { href: '/admin/scan', icon: 'refresh-circle', titleKey: 'home.admin.link.library.title', subtitleKey: 'home.admin.link.library.subtitle', color: '#f59e0b' },
  {
    href: '/admin/tracks',
    icon: 'musical-notes',
    titleKey: 'home.admin.link.tracks.title',
    subtitleKey: 'home.admin.link.tracks.subtitle',
    color: '#1ed760',
    requires: 'libraryAdmin',
  },
  {
    href: '/admin/providers',
    icon: 'cube',
    titleKey: 'home.admin.link.providers.title',
    subtitleKey: 'home.admin.link.providers.subtitle',
    color: '#8b5cf6',
    requires: 'dynamicProviders',
  },
  {
    href: '/admin/radio',
    icon: 'radio',
    titleKey: 'home.admin.link.radio.title',
    subtitleKey: 'home.admin.link.radio.subtitle',
    color: '#ec4899',
    requires: 'internetRadio',
  },
  {
    href: '/admin/federation',
    icon: 'git-network',
    titleKey: 'home.admin.link.federation.title',
    subtitleKey: 'home.admin.link.federation.subtitle',
    color: '#14b8a6',
    requires: 'federation',
  },
  {
    href: '/admin/settings',
    icon: 'options',
    titleKey: 'home.admin.link.settings.title',
    subtitleKey: 'home.admin.link.settings.subtitle',
    color: '#0ea5e9',
    requires: 'runtimeSettings',
  },
];
