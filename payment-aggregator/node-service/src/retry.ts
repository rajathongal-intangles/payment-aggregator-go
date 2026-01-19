import * as grpc from '@grpc/grpc-js';

export interface RetryConfig {
  maxRetries: number;
  initialBackoffMs: number;
  maxBackoffMs: number;
  backoffFactor: number;
  jitter: boolean;
}

export const defaultRetryConfig: RetryConfig = {
  maxRetries: 3,
  initialBackoffMs: 100,
  maxBackoffMs: 10000,
  backoffFactor: 2,
  jitter: true,
};

export async function withRetry<T>(
  fn: () => Promise<T>,
  config: RetryConfig = defaultRetryConfig
): Promise<T> {
  let lastError: Error | undefined;

  for (let attempt = 0; attempt <= config.maxRetries; attempt++) {
    try {
      return await fn();
    } catch (err) {
      lastError = err as Error;

      if (!isRetryable(err)) {
        throw err;
      }

      if (attempt === config.maxRetries) {
        break;
      }

      const backoff = calculateBackoff(attempt, config);
      console.log(`🔄 Retry attempt ${attempt + 1}/${config.maxRetries} in ${backoff}ms...`);
      await sleep(backoff);
    }
  }

  throw lastError;
}

function calculateBackoff(attempt: number, config: RetryConfig): number {
  let backoff = config.initialBackoffMs * Math.pow(config.backoffFactor, attempt);
  backoff = Math.min(backoff, config.maxBackoffMs);

  if (config.jitter) {
    const jitter = backoff * 0.25 * (Math.random() * 2 - 1);
    backoff += jitter;
  }

  return Math.floor(backoff);
}

function isRetryable(err: any): boolean {
  const retryableCodes = [
    grpc.status.UNAVAILABLE,      // 14
    grpc.status.ABORTED,          // 10
    grpc.status.RESOURCE_EXHAUSTED, // 8
    grpc.status.DEADLINE_EXCEEDED,  // 4
  ];
  return retryableCodes.includes(err?.code);
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// Error classification utilities
export function classifyError(err: any): {
  retryable: boolean;
  code: number;
  message: string;
  name: string;
} {
  const code = err?.code ?? grpc.status.UNKNOWN;
  const message = err?.message ?? 'Unknown error';

  const retryableCodes = [
    grpc.status.UNAVAILABLE,
    grpc.status.ABORTED,
    grpc.status.RESOURCE_EXHAUSTED,
    grpc.status.DEADLINE_EXCEEDED,
  ];

  return {
    retryable: retryableCodes.includes(code),
    code,
    message,
    name: getErrorName(code),
  };
}

export function getErrorName(code: number): string {
  const names: Record<number, string> = {
    0: 'OK',
    1: 'CANCELLED',
    2: 'UNKNOWN',
    3: 'INVALID_ARGUMENT',
    4: 'DEADLINE_EXCEEDED',
    5: 'NOT_FOUND',
    6: 'ALREADY_EXISTS',
    7: 'PERMISSION_DENIED',
    8: 'RESOURCE_EXHAUSTED',
    9: 'FAILED_PRECONDITION',
    10: 'ABORTED',
    11: 'OUT_OF_RANGE',
    12: 'UNIMPLEMENTED',
    13: 'INTERNAL',
    14: 'UNAVAILABLE',
    15: 'DATA_LOSS',
    16: 'UNAUTHENTICATED',
  };
  return names[code] ?? 'UNKNOWN';
}
