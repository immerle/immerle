import {
  createImmerleApi,
  FieldErrorDTO,
  SetupInitRequest,
  SetupStatus,
} from './immerleApi';

/**
 * First-run setup calls, built on the generated OpenAPI client. These two
 * endpoints are unauthenticated (the server self-locks once an admin exists)
 * and live under `/api/v1` — e.g. `GET <server>/api/v1/setup`.
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
  const { data, error } = await api.GET('/setup', { signal });
  if (error || !data) throw new Error('setup_status_failed');
  return data;
}

export async function initSetup(serverUrl: string, payload: InitPayload): Promise<InitResult> {
  const api = createImmerleApi(serverUrl);
  const { data, error, response } = await api.POST('/setup', { body: payload });

  if (response.status === 201 && data) {
    return { ok: true, user: data };
  }
  // Non-2xx bodies (validation/token/conflict) carry {error:{code,message,fields}}.
  const body = (error ?? {}) as { error?: { code?: string; fields?: FieldErrorDTO[] } };
  return {
    ok: false,
    status: response.status,
    error: body.error?.code ?? 'error',
    details: body.error?.fields,
  };
}
