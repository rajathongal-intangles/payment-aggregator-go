# Payment Aggregator - gRPC Kafka Sidecar Pattern

A proof-of-concept demonstrating how a **Go gRPC sidecar** can efficiently consume multiple Kafka topics and stream events to Node.js (or any language) clients.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Kafka Cluster                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                   │
│  │   payments   │  │    orders    │  │    users     │  ...              │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘                   │
└─────────┼─────────────────┼─────────────────┼───────────────────────────┘
          │                 │                 │
          └────────────┬────┴─────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     Go Sidecar (Single Process)                          │
│                                                                          │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │              Consumer Manager (On-Demand)                        │   │
│   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │   │
│   │  │ Goroutine 1 │  │ Goroutine 2 │  │ Goroutine 3 │  ...         │   │
│   │  │ payments    │  │ orders      │  │ users       │              │   │
│   │  │ consumer    │  │ consumer    │  │ consumer    │              │   │
│   │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘              │   │
│   └─────────┼────────────────┼────────────────┼─────────────────────┘   │
│             │                │                │                          │
│             └────────┬───────┴────────────────┘                          │
│                      ▼                                                   │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │                    gRPC Server (:50051)                          │   │
│   │  • StreamPayments(topic) - Server streaming                      │   │
│   │  • GetPayment(id)        - Unary RPC                            │   │
│   │  • ListPayments(filter)  - Unary RPC                            │   │
│   │  • GetTopics()           - List active topics                   │   │
│   └──────────────────────────┬──────────────────────────────────────┘   │
│                              │                                           │
│   Resilience Patterns:       │                                           │
│   • Circuit Breaker          │                                           │
│   • Retry + Exponential Backoff                                          │
│   • Dead Letter Queue (DLQ)  │                                           │
└──────────────────────────────┼──────────────────────────────────────────┘
                               │ gRPC Streams
          ┌────────────────────┼────────────────────┐
          │                    │                    │
          ▼                    ▼                    ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│  Node.js Client  │ │  Node.js Client  │ │  Node.js Client  │
│  (payments)      │ │  (orders)        │ │  (users)         │
└──────────────────┘ └──────────────────┘ └──────────────────┘
```

## Why This Pattern?

### The Problem
- Node.js (and many languages) have limited or complex Kafka client libraries
- Each service duplicates Kafka connection logic, error handling, retries
- No centralized observability for message consumption
- Difficult to implement consistent resilience patterns across services

### The Solution: Go Sidecar
A single Go process acts as a **Kafka consumption proxy**:

| Benefit | Description |
| ------- | ----------- |
| **Language Agnostic** | Any language with gRPC support can consume Kafka events |
| **Centralized Resilience** | Circuit breaker, retries, DLQ in one place |
| **Efficient Concurrency** | Go handles 10-50+ topics in one process via goroutines |
| **On-Demand Consumers** | Consumers created when clients subscribe, destroyed when they leave |
| **Type Safety** | Protobuf schema ensures consistent message format |

## Why Go for the Sidecar?

Go's concurrency model makes it ideal for this pattern:

```
Goroutine Memory: ~2KB stack (vs ~1MB for OS threads)
Scheduling:       Go runtime multiplexes across OS threads
I/O Model:        Non-blocking - Kafka polling doesn't block threads
Result:           One process handles many topics efficiently
```

### Capacity Estimate

A single Go sidecar can typically handle:
- **10-50+ Kafka topics** concurrently
- **Thousands of messages/second** (I/O bound, not CPU bound)
- **Limited by**: Network bandwidth, Kafka cluster, message processing complexity

## Key Features

### On-Demand Consumer Management

Consumers are created dynamically when clients subscribe:

```
Client connects → StreamPayments("payments") → Consumer created for "payments"
Another client  → StreamPayments("payments") → Shares existing consumer
Client disconnects                           → If last subscriber, consumer destroyed
```

This means:
- No idle consumers for unused topics
- Resources allocated only when needed
- Automatic cleanup

### Circuit Breaker

Prevents cascade failures when downstream systems are unavailable.

```
States: CLOSED → OPEN → HALF-OPEN → CLOSED

CLOSED:     Normal operation, requests pass through
            → 5 failures → OPEN

OPEN:       Requests fail immediately (no load on failing system)
            → 30 seconds → HALF-OPEN

HALF-OPEN:  Allow limited requests to test recovery
            → 2 successes → CLOSED
            → 1 failure → OPEN
```

### Retry with Exponential Backoff + Jitter

```
Attempt 1: Wait ~100ms  (100 * 2^0 ± 25% jitter)
Attempt 2: Wait ~200ms  (100 * 2^1 ± 25% jitter)
Attempt 3: Wait ~400ms  (100 * 2^2 ± 25% jitter)
Max:       3 retries, then send to DLQ
```

### Dead Letter Queue (DLQ)

Failed messages are sent to `{topic}.dlq` with metadata:

```json
{
  "original_topic": "poc.kafka.go.side.car.payments",
  "partition": 0,
  "offset": 12345,
  "key": "pay_abc123",
  "value": "{original message}",
  "error": "max_retries_exceeded",
  "retry_count": 3,
  "timestamp": 1705123456
}
```

## Quick Start

### Prerequisites
- Go 1.21+
- Node.js 18+
- Kafka (local or cloud)
- protoc (Protocol Buffers compiler)

### Setup

```bash
# Install dependencies and generate proto
make setup
```

### Create Kafka Topics

Create these topics in your Kafka cluster before testing:

```bash
# Main topics
poc.kafka.go.side.car.payments
poc.kafka.go.side.car.orders
poc.kafka.go.side.car.users

# DLQ topics (auto-created by producer, but you can pre-create)
poc.kafka.go.side.car.payments.dlq
poc.kafka.go.side.car.orders.dlq
poc.kafka.go.side.car.users.dlq
```

**Using Confluent Cloud CLI:**
```bash
confluent kafka topic create poc.kafka.go.side.car.payments --partitions 3
confluent kafka topic create poc.kafka.go.side.car.orders --partitions 3
confluent kafka topic create poc.kafka.go.side.car.users --partitions 3
```

**Using local Kafka:**
```bash
kafka-topics.sh --create --topic poc.kafka.go.side.car.payments --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1
kafka-topics.sh --create --topic poc.kafka.go.side.car.orders --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1
kafka-topics.sh --create --topic poc.kafka.go.side.car.users --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1
```

### Basic Test (Single Topic)

**Using Makefile:**
```bash
# Terminal 1: Start Go server
make server

# Terminal 2: Start Node.js client
make client

# Terminal 3: Send messages
make producer
```

**Using run.sh:**
```bash
# Terminal 1: Start Go server
./run.sh server

# Terminal 2: Start Node.js client
./run.sh client

# Terminal 3: Send messages
./run.sh producer 10
```

## Multi-Topic Load Test

Test parallel consumption from 3 topics with DLQ.

### Run the Test (5 Terminals)

**Using Makefile:**
```bash
# Terminal 1: Start Go sidecar server
make server

# Terminal 2: Client for payments topic
make client-payments

# Terminal 3: Client for orders topic
make client-orders

# Terminal 4: Client for users topic
make client-users

# Terminal 5: Blast 100 valid + 10 invalid messages per topic (no delay)
make multi-topic
```

**Using run.sh:**
```bash
# Terminal 1: Start Go sidecar server
./run.sh server

# Terminal 2: Client for payments topic
./run.sh client poc.kafka.go.side.car.payments

# Terminal 3: Client for orders topic
./run.sh client poc.kafka.go.side.car.orders

# Terminal 4: Client for users topic
./run.sh client poc.kafka.go.side.car.users

# Terminal 5: Blast 100 valid + 10 invalid messages per topic (no delay)
./run.sh multi-topic 100 10
```

### Difference: Makefile vs run.sh

| | **Makefile** | **run.sh** |
|---|---|---|
| Syntax | `make server` | `./run.sh server` |
| Topic arg | Fixed targets | Accepts any topic name |
| Best for | Quick commands, CI/CD | Ad-hoc testing, custom topics |

Use `run.sh` for custom topics: `./run.sh client my-custom-topic`

### What to Expect

**Server logs:**
```
[MANAGER] Created new consumer for topic: poc.kafka.go.side.car.payments
[MANAGER] Created new consumer for topic: poc.kafka.go.side.car.orders
[MANAGER] Created new consumer for topic: poc.kafka.go.side.car.users
[CONSUMER:poc.kafka.go.side.car.payments] pay_123 | stripe | 45.99 USD
[CONSUMER:poc.kafka.go.side.car.orders] pay_456 | razorpay | 1500.00 INR
...
[CONSUMER:poc.kafka.go.side.car.payments] Retry 1/3 in 100ms
[CONSUMER:poc.kafka.go.side.car.payments] Max retries exceeded, sending to DLQ
[DLQ:poc.kafka.go.side.car.payments] Sent: invalid-poc.kafka.go.side.car.payments-0
```

**Client logs:**
```
Connected to gRPC server at localhost:50051
Subscribing to topic: poc.kafka.go.side.car.payments
🆕 [poc.kafka.go.side.car.payments] pay_123 | STRIPE | $45.99 USD | COMPLETED
📦 [poc.kafka.go.side.car.payments] pay_456 | RAZORPAY | ₹1500.00 INR | PENDING
```

## DLQ Testing

Test the Dead Letter Queue with invalid messages:

```bash
# Quick test: 2 valid + 3 invalid
make dlq-test

# Or custom:
./run.sh producer 5 3   # 5 valid + 3 invalid
```

Invalid messages trigger:
1. Parse failure (invalid JSON or missing required fields)
2. 3 retry attempts with exponential backoff
3. Send to DLQ topic after max retries

## Configuration

### Environment Variables

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka bootstrap servers |
| `KAFKA_TOPIC` | `poc.kafka.go.side.car` | Default topic |
| `KAFKA_GROUP_ID` | `payment-service` | Consumer group ID |
| `KAFKA_SSL` | `false` | Enable SSL/TLS |
| `KAFKA_SASL_MECHANISM` | `PLAIN` | SASL mechanism |
| `KAFKA_SASL_USERNAME` | - | SASL username |
| `KAFKA_SASL_PASSWORD` | - | SASL password |
| `GRPC_PORT` | `50051` | gRPC server port |

### Example .env

```bash
# Kafka
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=poc.kafka.go.side.car
KAFKA_GROUP_ID=payment-service

# gRPC
GRPC_PORT=50051

# For cloud Kafka (e.g., Confluent Cloud)
# KAFKA_SSL=true
# KAFKA_SASL_MECHANISM=PLAIN
# KAFKA_SASL_USERNAME=your-api-key
# KAFKA_SASL_PASSWORD=your-api-secret
```

## Project Structure

```
payment-aggregator/
├── go-service/
│   ├── cmd/
│   │   ├── server/main.go      # Entry point - starts gRPC server
│   │   └── producer/main.go    # Multi-topic test producer
│   ├── internal/
│   │   ├── grpc/server.go      # PaymentService implementation
│   │   ├── kafka/
│   │   │   ├── manager.go      # On-demand consumer manager
│   │   │   ├── consumer.go     # Single consumer with DLQ
│   │   │   ├── producer.go     # Producer helper
│   │   │   └── message.go      # JSON → Protobuf + validation
│   │   ├── circuit/breaker.go  # Circuit breaker state machine
│   │   ├── retry/retry.go      # Retry with exponential backoff
│   │   ├── errors/errors.go    # AppError → gRPC status mapping
│   │   └── config/config.go    # Environment config loader
│   ├── proto/payment.proto     # Service definition
│   └── pb/                     # Generated protobuf code
│
├── node-service/
│   ├── src/
│   │   ├── client.ts           # PaymentClient with streaming + reconnect
│   │   ├── index.ts            # Main entry point (accepts topic arg)
│   │   ├── retry.ts            # Error classification + retry logic
│   │   ├── logger.ts           # Payment event formatter
│   │   └── config.ts           # Config loader
│   └── proto/payment.proto     # Proto (same as Go)
│
├── Makefile                    # Build commands
├── run.sh                      # Quick start script
└── README.md                   # This file
```

## gRPC API

### StreamPayments (Server Streaming)

```protobuf
rpc StreamPayments(StreamRequest) returns (stream PaymentEvent);

message StreamRequest {
    string topic = 1;           // Kafka topic to subscribe (required)
    Provider provider = 2;      // Filter (optional)
    PaymentStatus status = 3;   // Filter (optional)
}

message PaymentEvent {
    Payment payment = 1;
    string event_type = 2;      // "existing" or "new"
    string topic = 3;           // Source topic
}
```

### GetTopics

```protobuf
rpc GetTopics(GetTopicsRequest) returns (TopicList);

message TopicList {
    repeated string topics = 1;  // Active topics with consumers
    int32 count = 2;
}
```

### GetPayment / ListPayments

```protobuf
rpc GetPayment(GetPaymentRequest) returns (Payment);
rpc ListPayments(ListPaymentsRequest) returns (PaymentList);
```

## Makefile Commands

```bash
make setup          # Install deps + generate proto
make server         # Start Go gRPC server
make client         # Start Node.js client (default topic)
make client-payments # Client for payments topic
make client-orders   # Client for orders topic
make client-users    # Client for users topic
make producer       # Send 10 messages to default topic
make dlq-test       # Send 2 valid + 3 invalid (DLQ test)
make multi-topic    # 100 valid + 10 invalid to 3 topics (no delay)
make clean          # Remove generated files
```

## Scaling Strategies

### Primary: Single Sidecar, Multiple Topics

```
One Go Sidecar handles all topics via goroutines:
├─ Goroutine: payments consumer
├─ Goroutine: orders consumer
├─ Goroutine: users consumer
└─ ... (10-50+ topics easily)
```

### When to Add More Sidecars

| Scenario | Solution |
| -------- | -------- |
| Machine CPU/memory maxed | Add sidecar on another machine |
| Single high-throughput topic | Multiple consumers in same group |
| Fault isolation needed | Dedicated sidecar for critical topics |
| Geographic distribution | Regional sidecars near Kafka clusters |

## Future Enhancements

- [ ] Prometheus metrics for consumer lag, message rates, circuit breaker state
- [ ] OpenTelemetry distributed tracing
- [ ] Persistent storage (Redis/PostgreSQL) instead of in-memory
- [ ] Admin API for consumer management
- [ ] Health checks and readiness probes

## License

MIT
