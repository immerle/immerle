import { ScrollView, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useDownloadJobs, useJobMutations } from '../../src/query/admin';
import { DownloadJob, DownloadJobStatus } from '../../src/api/gossignol/types';
import { Badge, Card, EmptyState, ErrorState, IconButton, Loading } from '../../src/components/ui';
import { useColors } from '../../src/theme/colors';

const STATUS_TONE: Record<DownloadJobStatus, 'default' | 'success' | 'danger' | 'primary'> = {
  queued: 'default',
  running: 'primary',
  completed: 'success',
  failed: 'danger',
  cancelled: 'default',
};
const STATUS_LABEL: Record<DownloadJobStatus, string> = {
  queued: 'En attente',
  running: 'En cours',
  completed: 'Terminé',
  failed: 'Échec',
  cancelled: 'Annulé',
};

/** Download job queue: live progress, retry failed/cancelled, cancel running. */
export default function AdminJobs() {
  const colors = useColors();
  const q = useDownloadJobs();
  const { retry, cancel } = useJobMutations();

  return (
    <>
      <Stack.Screen options={{ title: 'File de téléchargement' }} />
      {q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message="Impossible de charger la file." onRetry={q.refetch} />
      ) : !q.data?.length ? (
        <EmptyState icon="cloud-download" title="File vide" subtitle="Aucun téléchargement en cours." />
      ) : (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ padding: 16, gap: 12 }}>
          {q.data.map((job) => (
            <JobCard
              key={job.id}
              job={job}
              onRetry={() => retry.mutate(job.id)}
              onCancel={() => cancel.mutate(job.id)}
            />
          ))}
        </ScrollView>
      )}
    </>
  );
}

function JobCard({ job, onRetry, onCancel }: { job: DownloadJob; onRetry: () => void; onCancel: () => void }) {
  const colors = useColors();
  const active = job.status === 'running' || job.status === 'queued';
  const canRetry = job.status === 'failed' || job.status === 'cancelled';

  return (
    <Card className="gap-2">
      <View className="flex-row items-center justify-between">
        <Text numberOfLines={1} className="flex-1 text-base font-semibold text-foreground">
          {job.title || job.query}
        </Text>
        <Badge label={STATUS_LABEL[job.status]} tone={STATUS_TONE[job.status]} />
      </View>
      {job.artist ? <Text className="text-sm text-muted">{job.artist}</Text> : null}

      {job.status === 'running' ? (
        <View className="h-1.5 w-full overflow-hidden rounded-full bg-surface-alt">
          <View className="h-full bg-primary" style={{ width: `${Math.round(job.progress * 100)}%` }} />
        </View>
      ) : null}

      {job.error ? <Text className="text-xs text-danger">{job.error}</Text> : null}

      <View className="flex-row justify-end gap-1">
        {canRetry ? (
          <IconButton name="refresh" size={22} color={colors.primary} onPress={onRetry} accessibilityLabel="Relancer" />
        ) : null}
        {active ? (
          <IconButton name="close-circle" size={22} color={colors.danger} onPress={onCancel} accessibilityLabel="Annuler" />
        ) : null}
      </View>
    </Card>
  );
}
