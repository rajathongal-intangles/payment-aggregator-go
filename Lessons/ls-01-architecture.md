# Lesson 1: Architecture & Why gRPC?

> **Progress**: `[█░░░░░░░░░]` 10% - Foundation  
> **Time**: ~10 minutes

---

## 🎯 Learning Objectives

- Understand the system we're building
- Know why gRPC over REST for this use case
- Understand Protocol Buffers basics

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        PAYMENT GATEWAY AGGREGATOR                           │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────┐      ┌──────────────────┐      ┌──────────────────────────┐
│              │      │                  │      │                          │
│  Go Producer │─────▶│  Confluent Kafka │─────▶│  Go Consumer + gRPC Srv  │
│  (test data) │      │  (cloud)         │      │  (normalize + serve)     │
│              │      │                  │      │                          │
└──────────────┘      └──────────────────┘      └────────────┬─────────────┘
                                                             │
                                                             │ gRPC (HTTP/2)
                                                             │
                                                             ▼
                                               ┌──────────────────────────┐
                                               │                          │
                                               │  Node.js gRPC Client     │
                                               │  (process + log)         │
                                               │                          │
                                               └──────────────────────────┘
```

### Components We'll Build

| Component | Language | Purpose |
|-----------|----------|---------|
| **Producer** | Go | Generate fake payment events → Kafka |
| **Consumer + gRPC Server** | Go | Consume from Kafka, normalize, expose via gRPC |
| **gRPC Client** | Node.js | Request payments via gRPC, process & log |

---

## Why gRPC Instead of REST?

```
REST (HTTP/1.1 + JSON)          vs          gRPC (HTTP/2 + Protobuf)
─────────────────────────────────────────────────────────────────────
        ┌─────────┐                              ┌─────────┐
        │ {"id":  │                              │ binary  │
        │  "123", │  ← Text, human readable      │ 0x0a03  │ ← Binary, compact
        │  "amt": │    but verbose               │ 3132... │   ~10x smaller
        │  50.00} │                              │         │
        └─────────┘                              └─────────┘
              │                                        │
              ▼                                        ▼
        Manual type                              Auto-generated
        validation                               type-safe code
```

### Comparison Table

| Aspect | REST | gRPC |
|--------|------|------|
| Protocol | HTTP/1.1 (text) | HTTP/2 (binary) |
| Payload | JSON | Protocol Buffers |
| Speed | Slower | ~10x faster |
| Streaming | Workarounds | Built-in |
| Type Safety | Manual | Auto-generated |
| Contract | OpenAPI (optional) | `.proto` (required) |

### For Our Payment System

- ✅ **High throughput** - Thousands of transactions/second
- ✅ **Type safety** - Money handling needs strict types
- ✅ **Polyglot** - Go ↔ Node.js with same contract

---

## What is Protocol Buffers?

A **schema + serialization format** in one file.

```
                    ┌─────────────────────────────┐
                    │      payment.proto          │
                    │  ┌───────────────────────┐  │
                    │  │ message Payment {     │  │
                    │  │   string id = 1;      │  │
                    │  │   double amount = 2;  │  │
                    │  │   string currency = 3;│  │
                    │  │ }                     │  │
                    │  └───────────────────────┘  │
                    └──────────────┬──────────────┘
                                   │
                                   ▼
                          protoc compiler
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
           ┌──────────────┐              ┌──────────────┐
           │   Go Code    │              │  Node.js     │
           │ payment.pb.go│              │ payment_pb.js│
           │ (auto-gen)   │              │ (auto-gen)   │
           └──────────────┘              └──────────────┘
```

> 💡 **Key insight**: Define the contract ONCE in `.proto`, both languages get type-safe code automatically.

---

## Data Flow

```
Step 1: Producer generates fake payment
────────────────────────────────────────
{
  "id": "pay_abc123",
  "provider": "stripe",
  "amount": 5000,        ← cents (provider format)
  "currency": "usd",
  "status": "succeeded"
}
        │
        ▼ Kafka Topic: payments.raw
        
Step 2: Go Consumer normalizes
────────────────────────────────────────
{
  "id": "pay_abc123",
  "provider": "STRIPE",
  "amount": 50.00,       ← dollars (normalized)
  "currency": "USD",
  "status": "COMPLETED",
  "processed_at": "2024-..."
}
        │
        ▼ gRPC Server exposes
        
Step 3: Node.js requests & logs
────────────────────────────────────────
[LOG] Payment received: pay_abc123 | $50.00 USD | STRIPE | COMPLETED
```

---

## Project Structure

```
payment-aggregator/
│
├── proto/                      # Shared contract (source of truth)
│   └── payment.proto
│
├── go-service/
│   ├── cmd/
│   │   ├── producer/          # Fake data generator
│   │   │   └── main.go
│   │   └── server/            # Consumer + gRPC server
│   │       └── main.go
│   ├── internal/
│   │   ├── kafka/             # Kafka producer & consumer
│   │   ├── normalizer/        # Data normalization
│   │   └── grpc/              # gRPC server implementation
│   ├── pb/                    # Generated protobuf code
│   └── go.mod
│
├── node-service/
│   ├── src/
│   │   ├── client.ts          # gRPC client
│   │   └── index.ts           # Entry point
│   ├── pb/                    # Generated protobuf code
│   └── package.json
│
└── README.md                   # Setup instructions
```

---
