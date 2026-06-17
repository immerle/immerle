import { useState } from 'react';
import { Alert, Platform, Pressable, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useUsers, useUserMutations } from '../../src/query/admin';
import { SubsonicUser } from '../../src/api/subsonic/types';
import { Badge, Button, Card, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle, colorFor } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';

/** Admin users: list, create, toggle admin role, reset password, delete. */
export default function AdminUsers() {
  const colors = useColors();
  const q = useUsers();
  const { create, update, remove, resetPassword } = useUserMutations();

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({ username: '', password: '', displayName: '', email: '', admin: false });
  const [expanded, setExpanded] = useState<string | null>(null);
  const [resetting, setResetting] = useState<string | null>(null);
  const [newPassword, setNewPassword] = useState('');

  const submitCreate = () => {
    if (!form.username.trim() || !form.password) return;
    create.mutate(
      {
        username: form.username.trim(),
        password: form.password,
        displayName: form.displayName.trim() || undefined,
        email: form.email || undefined,
        adminRole: form.admin,
      },
      {
        onSuccess: () => {
          setForm({ username: '', password: '', displayName: '', email: '', admin: false });
          setShowCreate(false);
        },
      },
    );
  };

  const confirmDelete = (username: string) => {
    const doDelete = () => remove.mutate(username);
    if (Platform.OS === 'web') doDelete();
    else
      Alert.alert('Supprimer cet utilisateur ?', username, [
        { text: 'Annuler', style: 'cancel' },
        { text: 'Supprimer', style: 'destructive', onPress: doDelete },
      ]);
  };

  const submitReset = (username: string) => {
    if (!newPassword) return;
    resetPassword.mutate(
      { username, password: newPassword },
      {
        onSuccess: () => {
          setResetting(null);
          setNewPassword('');
        },
      },
    );
  };

  const toggleRow = (username: string) => {
    setExpanded((cur) => (cur === username ? null : username));
    setResetting(null);
    setNewPassword('');
  };

  const users = q.data ?? [];

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      {q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message="Impossible de charger les utilisateurs." onRetry={q.refetch} />
      ) : (
        <AdminScroll
          header={
            <AdminHeader
              color="#3b82f6"
              title="Utilisateurs"
              subtitle={`${users.length} compte${users.length > 1 ? 's' : ''}`}
              trailing={
                <IconButton
                  name={showCreate ? 'close' : 'person-add'}
                  color={colors.primary}
                  onPress={() => setShowCreate((s) => !s)}
                />
              }
            />
          }
        >
          {showCreate ? (
            <Card className="gap-3">
              <CardTitle icon="person-add" color="#3b82f6" title="Nouvel utilisateur" />
              <Field label="Identifiant" placeholder="utilisateur" autoCapitalize="none" value={form.username} onChangeText={(v) => setForm({ ...form, username: v })} />
              <Field label="Nom affiché (optionnel)" placeholder="Jean Dupont" value={form.displayName} onChangeText={(v) => setForm({ ...form, displayName: v })} />
              <Field label="Mot de passe" secureTextEntry value={form.password} onChangeText={(v) => setForm({ ...form, password: v })} />
              <Field label="Email (optionnel)" keyboardType="email-address" autoCapitalize="none" value={form.email} onChangeText={(v) => setForm({ ...form, email: v })} />
              <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
                <Text className="text-sm text-foreground">Administrateur</Text>
                <Switch value={form.admin} onValueChange={(v) => setForm({ ...form, admin: v })} trackColor={{ true: colors.primary, false: colors.border }} />
              </View>
              <Button title="Créer l'utilisateur" icon="checkmark" loading={create.isPending} onPress={submitCreate} />
            </Card>
          ) : null}

          <Card className="p-0">
            {users.map((user, i) => (
              <UserRow
                key={user.username}
                user={user}
                first={i === 0}
                expanded={expanded === user.username}
                onToggle={() => toggleRow(user.username)}
                onToggleAdmin={(admin) => update.mutate({ username: user.username, adminRole: admin })}
                onSaveDisplayName={(name) => update.mutate({ username: user.username, displayName: name.trim() })}
                savingDisplayName={update.isPending}
                onDelete={() => confirmDelete(user.username)}
                resetting={resetting === user.username}
                onStartReset={() => setResetting(user.username)}
                onCancelReset={() => setResetting(null)}
                newPassword={newPassword}
                onChangePassword={setNewPassword}
                onSubmitReset={() => submitReset(user.username)}
                resetLoading={resetPassword.isPending}
              />
            ))}
          </Card>
        </AdminScroll>
      )}
    </>
  );
}

interface UserRowProps {
  user: SubsonicUser;
  first: boolean;
  expanded: boolean;
  onToggle: () => void;
  onToggleAdmin: (admin: boolean) => void;
  onSaveDisplayName: (name: string) => void;
  savingDisplayName: boolean;
  onDelete: () => void;
  resetting: boolean;
  onStartReset: () => void;
  onCancelReset: () => void;
  newPassword: string;
  onChangePassword: (v: string) => void;
  onSubmitReset: () => void;
  resetLoading: boolean;
}

function UserRow({
  user,
  first,
  expanded,
  onToggle,
  onToggleAdmin,
  onSaveDisplayName,
  savingDisplayName,
  onDelete,
  resetting,
  onStartReset,
  onCancelReset,
  newPassword,
  onChangePassword,
  onSubmitReset,
  resetLoading,
}: UserRowProps) {
  const colors = useColors();
  const isAdmin = !!user.adminRole;
  const shownName = user.displayName || user.username;
  const hasDisplay = !!user.displayName && user.displayName !== user.username;
  const [dn, setDn] = useState(user.displayName ?? '');
  return (
    <View className={first ? '' : 'border-t border-border'}>
      <Pressable onPress={onToggle} className="flex-row items-center gap-3 px-3 py-2.5 active:bg-surface-alt">
        <View className="h-10 w-10 items-center justify-center rounded-full" style={{ backgroundColor: colorFor(shownName) }}>
          <Text className="text-base font-bold text-white">{shownName.charAt(0).toUpperCase()}</Text>
        </View>
        <View className="flex-1">
          <View className="flex-row items-center gap-2">
            <Text className="text-base font-semibold text-foreground">{shownName}</Text>
            {isAdmin ? <Badge label="Admin" tone="success" /> : null}
          </View>
          <Text className="text-xs text-muted" numberOfLines={1}>
            {hasDisplay ? `@${user.username}${user.email ? ` · ${user.email}` : ''}` : user.email || 'Aucun email'}
          </Text>
        </View>
        <Ionicon name={expanded ? 'chevron-up' : 'chevron-down'} size={18} color={colors.muted} />
      </Pressable>

      {expanded ? (
        <View className="gap-2 px-3 pb-3">
          <View className="flex-row items-end gap-2">
            <View className="flex-1">
              <Field label="Nom affiché" placeholder={user.username} value={dn} onChangeText={setDn} />
            </View>
            <Button title="Enregistrer" size="sm" icon="checkmark" loading={savingDisplayName} onPress={() => onSaveDisplayName(dn)} />
          </View>

          <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
            <Text className="text-sm text-foreground">Rôle administrateur</Text>
            <Switch value={isAdmin} onValueChange={onToggleAdmin} trackColor={{ true: colors.primary, false: colors.border }} />
          </View>

          {resetting ? (
            <View className="gap-2">
              <Field label="Nouveau mot de passe" secureTextEntry value={newPassword} onChangeText={onChangePassword} />
              <View className="flex-row gap-2">
                <View className="flex-1">
                  <Button title="Annuler" size="sm" variant="ghost" onPress={onCancelReset} />
                </View>
                <View className="flex-1">
                  <Button title="Valider" size="sm" icon="checkmark" loading={resetLoading} onPress={onSubmitReset} />
                </View>
              </View>
            </View>
          ) : (
            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title="Réinitialiser le mot de passe" size="sm" icon="key-outline" variant="secondary" onPress={onStartReset} />
              </View>
              <Button title="Supprimer" size="sm" icon="trash-outline" variant="danger" onPress={onDelete} />
            </View>
          )}
        </View>
      ) : null}
    </View>
  );
}
