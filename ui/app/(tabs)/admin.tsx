import { Redirect } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAuth } from '../../src/auth/store';
import { EmptyState } from '../../src/components/ui';
import { ADMIN_LINKS } from '../../src/nav/adminLinks';
import { useT } from '../../src/i18n/store';

/**
 * /admin has no overview page: it lands straight on the first admin section —
 * Settings by default, falling back to the first available one. The admin
 * sidebar (desktop) / drawer (mobile) handles navigation from there.
 */
export default function Admin() {
  const t = useT();
  const client = useAuth((s) => s.client);

  if (!client?.isAdmin) {
    return (
      <SafeAreaView edges={['top']} className="flex-1 bg-background">
        <EmptyState icon="lock-closed" title={t('home.admin.restricted')} subtitle={t('home.admin.restrictedSubtitle')} />
      </SafeAreaView>
    );
  }

  const visible = ADMIN_LINKS.filter((l) => !l.requires || client.has(l.requires));
  const first = visible.find((l) => l.href === '/admin/settings')?.href ?? visible[0]?.href ?? '/admin/users';
  return <Redirect href={first as never} />;
}
