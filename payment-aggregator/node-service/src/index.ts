import { PaymentClient } from './client';
import { logPaymentSimple, PaymentEvent } from './logger';
import { config } from './config';

let stats = {
  existing: 0,
  new: 0,
  total: 0,
  startTime: Date.now(),
};

async function main() {
  const topic = process.argv[2] || config.kafka.topic;

  console.log(`
╔═══════════════════════════════════════════════════════════════╗
║           PAYMENT gRPC CLIENT (Node.js)                       ║
╠═══════════════════════════════════════════════════════════════╣
║  Server: ${config.grpc.address.padEnd(52)}║
║  Topic:  ${topic.padEnd(52)}║
║  Auto-reconnect: enabled                                      ║
╚═══════════════════════════════════════════════════════════════╝
`);

  const client = new PaymentClient();

  try {
    await client.connect();

    console.log(`\nSubscribing to topic: ${topic}\n`);
    console.log('─'.repeat(65));

    // Use reconnecting stream with topic
    client.streamPaymentsWithReconnect(
      topic,
      (event: PaymentEvent) => {
        stats.total++;
        if (event.eventType === 'new') stats.new++;
        else stats.existing++;
        logPaymentSimple(event);
      },
      (err: Error) => {
        console.error('\nFatal error:', err.message);
        printStats();
        process.exit(1);
      },
      {
        maxRetries: 10,
        retryDelayMs: 3000,
      }
    );

    // Graceful shutdown
    const shutdown = () => {
      console.log('\n\n⏳ Shutting down...');
      client.close();
      printStats();
      process.exit(0);
    };

    process.on('SIGINT', shutdown);
    process.on('SIGTERM', shutdown);

    console.log('Waiting for payments... (Ctrl+C to stop)\n');

  } catch (err) {
    console.error('❌ Failed to start:', err);
    process.exit(1);
  }
}

function printStats() {
  const duration = ((Date.now() - stats.startTime) / 1000).toFixed(1);
  console.log(`
┌─────────────────────────────────────────────────────────────┐
│                      SESSION STATS                          │
├─────────────────────────────────────────────────────────────┤
│  Total Payments:    ${String(stats.total).padEnd(38)}│
│  Existing:          ${String(stats.existing).padEnd(38)}│
│  New:               ${String(stats.new).padEnd(38)}│
│  Duration:          ${(duration + 's').padEnd(38)}│
└─────────────────────────────────────────────────────────────┘`);
}

main();