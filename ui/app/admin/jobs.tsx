import { ScrollView, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useDownloadJobs, useJobMutations } from '../../src/query/admin';
import { DownloadJob, DownloadJobStatus } from '../../src/api/immerle/types';
import { Badge, Card, EmptyState, ErrorState, IconButton, Loading } from '../../src/components/ui';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

const STATUS_TONE: Record<DownloadJobStatus, 'default' | 'success' | 'danger' | 'primary'> = {
  queued: 'default',
  running: 'primary',
  completed: 'success',
  failed: 'danger',
  cancelled: 'default',
};
const STATUS_LABEL_KEY: Record<DownloadJobStatus, string> = {
  queued: 'admin.jobs.statusQueued',
  running: 'admin.jobs.statusRunning',
  completed: 'admin.jobs.statusCompleted',
  failed: 'admin.jobs.statusFailed',
  cancelled: 'admin.jobs.statusCancelled',
};

/** Download job queue: live progress, retry failed/cancelled, cancel running. */
export default function AdminJobs() {
  const t = useT();
  const q = useDownloadJobs();
  const { retry, cancel } = useJobMutations();

  return (
    <>
      <Stack.Screen options={{ title: t('admin.jobs.title') }} />
      {q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message={t('admin.jobs.loadError')} onRetry={q.refetch} />
      ) : !q.data?.length ? (
        <EmptyState icon="cloud-download" title={t('admin.jobs.emptyTitle')} subtitle={t('admin.jobs.emptySubtitle')} />
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
  const t = useT();
  const colors = useColors();
  const active = job.status === 'running' || job.status === 'queued';
  const canRetry = job.status === 'failed' || job.status === 'cancelled';

  return (
    <Card className="gap-2">
      <View className="flex-row items-center justify-between">
        <Text numberOfLines={1} className="flex-1 text-base font-semibold text-foreground">
          {job.title || job.query}
        </Text>
        <Badge label={t(STATUS_LABEL_KEY[job.status])} tone={STATUS_TONE[job.status]} />
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
          <IconButton name="refresh" size={22} color={colors.primary} onPress={onRetry} accessibilityLabel={t('admin.jobs.retry')} />
        ) : null}
        {active ? (
          <IconButton name="close-circle" size={22} color={colors.danger} onPress={onCancel} accessibilityLabel={t('admin.jobs.cancel')} />
        ) : null}
      </View>
    </Card>
  );
}
