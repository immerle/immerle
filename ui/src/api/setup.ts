import {
  createImmerleApi,
  FieldErrorDTO,
  SetupInitRequest,
  SetupStatus,
} from './immerleApi';

/**
 * First-run setup calls, built on the generated OpenAPI client. These two
 * endpoints are unauthenticated (the server self-locks once an admin exists)
 * and live at the server root — e.g. `GET <server>/setup/status`.
 */

export type { SetupStatus };
export type SetupFieldError = FieldErrorDTO;
export type InitPayload = SetupInitRequest;

export type InitResult =
  | { ok: true; user: { id?: string; username?: string; isAdmin?: boolean } }
  | { ok: false; status: number; error: string; details?: SetupFieldError[] };

export async function getSetupStatus(
  serverUrl: string,
  signal?: AbortSignal,
): Promise<SetupStatus> {
  const api = createImmerleApi(serverUrl);
  const { data, error } = await api.GET('/setup/status', { signal });
  if (error || !data) throw new Error('setup_status_failed');
  return data;
}

export async function initSetup(serverUrl: string, payload: InitPayload): Promise<InitResult> {
  const api = createImmerleApi(serverUrl);
  const { data, error, response } = await api.POST('/setup/init', { body: payload });

  if (response.status === 201 && data?.user) {
    return { ok: true, user: data.user };
  }
  // Non-2xx bodies (validation/token/conflict) are returned in `error`.
  const body = (error ?? {}) as { error?: string; details?: FieldErrorDTO[] };
  return {
    ok: false,
    status: response.status,
    error: body.error ?? 'error',
    details: body.details,
  };
}
