import { useState } from 'react';
import { Alert, Platform, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useTokens, useTokenMutations } from '../src/query/account';
import { Badge, Button, Card, EmptyState, ErrorState, Field, IconButton, Loading } from '../src/components/ui';
import { Chip } from '../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle } from '../src/components/AdminUI';
import { Ionicon } from '../src/components/Ionicon';
import { APITokenDTO } from '../src/api/gossignolApi';
import { useColors } from '../src/theme/colors';

const DAY = 86_400_000;
const EXPIRIES: { key: string; label: string; ms: number }[] = [
  { key: 'never', label: 'Jamais', ms: 0 },
  { key: '30', label: '30 jours', ms: 30 * DAY },
  { key: '90', label: '90 jours', ms: 90 * DAY },
  { key: '365', label: '1 an', ms: 365 * DAY },
];

/**
 * Manage the current user's personal API tokens (extension API `/tokens`):
 * create (secret shown once), list, and revoke. Tokens authenticate API/CLI
 * access via `Authorization: Bearer <token>` or `?apiKey=<token>`.
 */
export default function ApiTokens() {
  const colors = useColors();
  const q = useTokens();
  const { create, revoke } = useTokenMutations();

  const [name, setName] = useState('');
  const [expiry, setExpiry] = useState('never');
  const [secret, setSecret] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const submit = () => {
    const preset = EXPIRIES.find((e) => e.key === expiry)!;
    const expires = preset.ms ? Date.now() + preset.ms : undefined;
    create.mutate(
      { name: name.trim() || undefined, expires },
      {
        onSuccess: (res) => {
          setSecret(res.token ?? null);
          setCopied(false);
          setName('');
        },
      },
    );
  };

  const copy = () => {
    if (!secret) return;
    const nav = globalThis.navigator as { clipboard?: { writeText?: (s: string) => Promise<void> } };
    void nav.clipboard?.writeText?.(secret);
    setCopied(true);
  };

  const confirmRevoke = (id: string, label: string) => {
    const doIt = () => revoke.mutate(id);
    if (Platform.OS === 'web') doIt();
    else
      Alert.alert('Révoquer ce token ?', label, [
        { text: 'Annuler', style: 'cancel' },
        { text: 'Révoquer', style: 'destructive', onPress: doIt },
      ]);
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color="#6366f1" title="API" subtitle="Tokens d'accès personnels" />}>
        <View className="flex-row items-start gap-2 rounded-xl bg-surface-alt p-3">
          <Ionicon name="key-outline" size={18} color={colors.muted} />
          <Text className="flex-1 text-xs text-muted">
            Les tokens permettent d'accéder à l'API en votre nom (en-tête{' '}
            <Text className="text-foreground">Authorization: Bearer …</Text> ou{' '}
            <Text className="text-foreground">?apiKey=…</Text>). Le secret n'est affiché qu'une fois.
          </Text>
        </View>

        {/* Secret reveal (shown once after creation) */}
        {secret ? (
          <Card className="gap-2 border border-primary">
            <View className="flex-row items-center gap-2">
              <Ionicon name="checkmark-circle" size={18} color={colors.success} />
              <Text className="flex-1 text-sm font-semibold text-foreground">
                Token créé — copiez-le maintenant
              </Text>
              <IconButton name="close" size={18} color={colors.muted} onPress={() => setSecret(null)} />
            </View>
            <Text selectable className="rounded-lg bg-surface-alt px-3 py-2 text-xs text-foreground">
              {secret}
            </Text>
            <Button
              title={copied ? 'Copié ✓' : 'Copier le token'}
              icon={copied ? 'checkmark' : 'copy-outline'}
              variant="secondary"
              onPress={copy}
            />
          </Card>
        ) : null}

        {/* Create */}
        <Card className="gap-3">
          <CardTitle icon="add-circle" color="#6366f1" title="Nouveau token" />
          <Field label="Nom (optionnel)" placeholder="mon-cli" value={name} onChangeText={setName} autoCapitalize="none" />
          <Text className="text-sm font-medium text-muted">Expiration</Text>
          <View className="flex-row flex-wrap gap-2">
            {EXPIRIES.map((e) => (
              <Chip key={e.key} label={e.label} active={expiry === e.key} onPress={() => setExpiry(e.key)} />
            ))}
          </View>
          <Button title="Créer le token" icon="add" loading={create.isPending} onPress={submit} />
        </Card>

        {/* List */}
        <Text className="px-1 pt-1 text-xs font-medium uppercase tracking-wider text-muted">Tokens actifs</Text>
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message="Impossible de charger les tokens." onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState icon="key-outline" title="Aucun token" subtitle="Créez-en un ci-dessus." />
        ) : (
          q.data.map((t: APITokenDTO) => <TokenRow key={t.id} token={t} onRevoke={() => confirmRevoke(t.id!, t.name || t.prefix || '')} loading={revoke.isPending} />)
        )}
      </AdminScroll>
    </>
  );
}

function TokenRow({ token, onRevoke, loading }: { token: APITokenDTO; onRevoke: () => void; loading: boolean }) {
  const colors = useColors();
  const expired = token.expiresAt ? new Date(token.expiresAt).getTime() < Date.now() : false;
  const fmt = (d?: string) => (d ? new Date(d).toLocaleDateString() : '—');
  return (
    <Card className="gap-2">
      <View className="flex-row items-center gap-2">
        <Ionicon name="key" size={18} color={colors.primary} />
        <Text className="flex-1 text-base font-semibold text-foreground">{token.name || 'Sans nom'}</Text>
        {expired ? <Badge label="Expiré" tone="danger" /> : null}
        <IconButton name="trash-outline" size={20} color={colors.danger} onPress={onRevoke} accessibilityLabel="Révoquer" />
      </View>
      <Text className="text-xs text-muted">
        {token.prefix ? `${token.prefix}… · ` : ''}créé le {fmt(token.createdAt)}
        {token.lastUsedAt ? ` · utilisé le ${fmt(token.lastUsedAt)}` : ' · jamais utilisé'}
        {token.expiresAt ? ` · expire le ${fmt(token.expiresAt)}` : ' · n’expire pas'}
      </Text>
    </Card>
  );
}
