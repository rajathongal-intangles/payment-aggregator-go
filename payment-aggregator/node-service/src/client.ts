import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import { config } from './config';
import { PaymentEvent } from './logger';
import { classifyError, withRetry } from './retry';

// Re-export error utilities for external use
export { classifyError, getErrorName } from './retry';

const PROTO_OPTIONS: protoLoader.Options = {
  keepCase: false,
  longs: String,
  enums: Number,
  defaults: true,
  oneofs: true,
};

const packageDefinition = protoLoader.loadSync(config.proto.path, PROTO_OPTIONS);
const protoDescriptor = grpc.loadPackageDefinition(packageDefinition) as any;
const PaymentService = protoDescriptor.payment.PaymentService;

export class PaymentClient {
  private client: any;
  private connected: boolean = false;
  private reconnecting: boolean = false;
  private stream: grpc.ClientReadableStream<PaymentEvent> | null = null;

  // Callbacks for streaming
  private onPaymentCallback?: (event: PaymentEvent) => void;
  private onErrorCallback?: (err: Error) => void;

  constructor() {
    this.createClient();
  }

  private createClient(): void {
    this.client = new PaymentService(
      config.grpc.address,
      grpc.credentials.createInsecure(),
      {
        'grpc.keepalive_time_ms': 10000,
        'grpc.keepalive_timeout_ms': 5000,
        'grpc.keepalive_permit_without_calls': 1,
      }
    );
  }

  async connect(timeoutMs: number = 5000): Promise<void> {
    return new Promise((resolve, reject) => {
      const deadline = Date.now() + timeoutMs;
      
      this.client.waitForReady(deadline, (err: Error | undefined) => {
        if (err) {
          reject(new Error(`Failed to connect: ${err.message}`));
        } else {
          this.connected = true;
          console.log(`✅ Connected to gRPC server at ${config.grpc.address}`);
          resolve();
        }
      });
    });
  }

  /**
   * Stream payments with automatic reconnection
   */
  streamPaymentsWithReconnect(
    onPayment: (event: PaymentEvent) => void,
    onError?: (err: Error) => void,
    options: { provider?: number; maxRetries?: number; retryDelayMs?: number } = {}
  ): void {
    const maxRetries = options.maxRetries ?? 10;
    const retryDelayMs = options.retryDelayMs ?? 3000;
    let retryCount = 0;

    this.onPaymentCallback = onPayment;
    this.onErrorCallback = onError;

    const attemptReconnect = () => {
      if (retryCount >= maxRetries) {
        console.error('❌ Max retries exceeded');
        this.onErrorCallback?.(new Error('Max retries exceeded'));
        return;
      }

      retryCount++;
      this.reconnecting = true;
      console.log(`🔄 Reconnecting in ${retryDelayMs/1000}s (attempt ${retryCount}/${maxRetries})...`);

      setTimeout(async () => {
        this.reconnecting = false;
        this.createClient();

        try {
          await this.connect();
          startStream();
        } catch (e: any) {
          console.error('❌ Reconnection failed:', e.message);
          // Keep trying until max retries
          attemptReconnect();
        }
      }, retryDelayMs);
    };

    const startStream = () => {
      if (this.reconnecting) return;

      this.stream = this.client.streamPayments({
        provider: options.provider || 0,
        status: 0,
        limit: 0,
      });

      this.stream!.on('data', (event: PaymentEvent) => {
        retryCount = 0; // Reset on successful data
        this.onPaymentCallback?.(event);
      });

      this.stream!.on('error', (err: Error) => {
        const classified = classifyError(err);

        // CANCELLED means intentional close
        if (classified.code === grpc.status.CANCELLED) {
          return;
        }

        console.error(`\n❌ Stream error [${classified.name}]:`, classified.message);

        // Attempt reconnection for recoverable errors
        if (classified.retryable ||
            classified.code === grpc.status.UNKNOWN ||
            classified.code === grpc.status.INTERNAL) {
          attemptReconnect();
        } else {
          this.onErrorCallback?.(err);
        }
      });

      this.stream!.on('end', () => {
        console.log('📪 Stream ended by server');
        // Server closed stream - try to reconnect
        if (!this.reconnecting) {
          attemptReconnect();
        }
      });
    };

    startStream();
  }

  /**
   * Get a single payment by ID (with retry)
   */
  async getPayment(paymentId: string): Promise<any> {
    return withRetry(() => {
      return new Promise((resolve, reject) => {
        this.client.getPayment({ paymentId }, (err: Error, response: any) => {
          if (err) {
            const classified = classifyError(err);
            console.log(`[getPayment] Error: ${classified.name} - ${classified.message}`);
            reject(err);
          } else {
            resolve(response);
          }
        });
      });
    });
  }

  /**
   * List payments (with retry)
   */
  async listPayments(options: { provider?: number; status?: number; limit?: number } = {}): Promise<any> {
    return withRetry(() => {
      return new Promise((resolve, reject) => {
        this.client.listPayments(
          { provider: options.provider || 0, status: options.status || 0, limit: options.limit || 10 },
          (err: Error, response: any) => {
            if (err) {
              const classified = classifyError(err);
              console.log(`[listPayments] Error: ${classified.name} - ${classified.message}`);
              reject(err);
            } else {
              resolve(response);
            }
          }
        );
      });
    });
  }

  /**
   * Cancel active stream
   */
  cancelStream(): void {
    if (this.stream) {
      this.stream.cancel();
      this.stream = null;
    }
  }

  /**
   * Close connection
   */
  close(): void {
    this.cancelStream();
    this.client.close();
    this.connected = false;
    console.log('👋 Disconnected from gRPC server');
  }
}