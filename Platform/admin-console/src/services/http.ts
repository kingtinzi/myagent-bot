export class AdminApiError extends Error {
  status: number;
  code: string;
  detail: string;

  constructor(status: number, message: string, detail = '') {
    super(message);
    this.name = 'AdminApiError';
    this.status = status;
    this.code = statusToErrorCode(status);
    this.detail = detail;
  }
}

export type RequestJsonOptions = {
  method?: string;
  body?: BodyInit | Record<string, unknown> | unknown[] | null;
  headers?: HeadersInit;
};

export type JsonResponse<T> = {
  data: T;
  status: number;
  revision: string | null;
};

function statusToErrorCode(status: number) {
  switch (status) {
    case 400:
      return 'bad_request';
    case 401:
      return 'unauthorized';
    case 403:
      return 'forbidden';
    case 404:
      return 'not_found';
    case 409:
      return 'conflict';
    case 412:
      return 'precondition_failed';
    case 428:
      return 'precondition_required';
    default:
      return status >= 500 ? 'server_error' : 'request_failed';
  }
}

function fallbackErrorMessage(status: number) {
  switch (status) {
    case 401:
      return '登录已失效，请重新登录后台。';
    case 403:
      return '当前管理员没有执行该操作的权限。';
    case 404:
      return '请求的后台资源不存在。';
    case 409:
      return '请求冲突，请刷新页面后重试。';
    case 412:
      return '配置已被其他管理员更新，请重新加载后再保存。';
    case 428:
      return '缺少配置版本，请刷新后重试。';
    default:
      return status >= 500 ? '后台服务暂时不可用，请稍后重试。' : '后台请求失败，请稍后重试。';
  }
}

async function parseResponseBody(response: Response) {
  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    return response.json();
  }

  const text = await response.text();
  return text.trim();
}

function normalizeRequestBody(body: RequestJsonOptions['body']) {
  if (body == null || typeof body === 'string' || body instanceof FormData || body instanceof URLSearchParams || body instanceof Blob) {
    return body;
  }

  return JSON.stringify(body);
}

function buildHeaders(body: RequestJsonOptions['body'], headers?: HeadersInit) {
  const nextHeaders = new Headers(headers);
  nextHeaders.set('Accept', 'application/json');

  if (body != null && !(body instanceof FormData) && !(body instanceof URLSearchParams) && !(body instanceof Blob) && !nextHeaders.has('Content-Type')) {
    nextHeaders.set('Content-Type', 'application/json');
  }

  return nextHeaders;
}

export async function requestJSON<T>(path: string, options: RequestJsonOptions = {}): Promise<JsonResponse<T>> {
  const response = await fetch(path, {
    method: options.method ?? 'GET',
    credentials: 'include',
    headers: buildHeaders(options.body, options.headers),
    body: normalizeRequestBody(options.body),
  });

  const revision = response.headers.get('x-resource-version') ?? response.headers.get('etag');
  const payload = await parseResponseBody(response);

  if (!response.ok) {
    const detail =
      typeof payload === 'string'
        ? payload
        : payload && typeof payload === 'object' && 'message' in payload && typeof payload.message === 'string'
          ? payload.message
          : '';

    throw new AdminApiError(response.status, detail || fallbackErrorMessage(response.status), detail);
  }

  return {
    data: payload as T,
    status: response.status,
    revision,
  };
}
