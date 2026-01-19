# Payment Gateway Aggregator

> Learn Go + gRPC by building a real payment processing pipeline

```
Go Producer → Kafka (Confluent) → Go Consumer/gRPC Server → Node.js Client
```

## Why Go can handle 10-50+ topics in a single process:                                                                                                
                                                                                                                                                      
  1. Goroutines are lightweight - Each goroutine uses ~2KB stack (vs ~1MB for OS threads)                                                             
  2. Go scheduler - Multiplexes thousands of goroutines across a few OS threads                                                                       
  3. Non-blocking I/O - Kafka consumers use efficient polling, not blocking threads                                                                   
  4. One consumer per topic = one goroutine - Trivial resource cost   

---

## 📚 Course Lessons

| # | Lesson | Status |
|---|--------|--------|
| 1 | [Architecture & Why gRPC](./lessons/01-architecture.md) | ✅ |
| 2 | Go Setup & Basics | ⬜ |
| 3 | Protocol Buffers Deep Dive | ⬜ |
| 4 | Your First gRPC Server | ⬜ |
| 5 | Kafka Consumer in Go | ⬜ |
| 6 | Node.js gRPC Client | ⬜ |
| 7 | Connecting Everything | ⬜ |
| 8 | Error Handling & Retries | ⬜ |
| 9 | Testing & Debugging | ⬜ |
| 10 | Production Tips | ⬜ |

---

## 🛠️ Prerequisites

### 1. Go (1.21+)

**macOS:**
```bash
brew install go
```

**Linux (Ubuntu/Debian):**
```bash
# Download from https://go.dev/dl/
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz

# Add to ~/.bashrc or ~/.zshrc
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

**Verify:**
```bash
go version
# go version go1.22.0 linux/amd64
```

### 2. Node.js (18+)

**Using nvm (recommended):**
```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash
nvm install 18
nvm use 18
```

**Verify:**
```bash
node --version
# v18.x.x
```

### 3. Protocol Buffer Compiler (protoc)

**macOS:**
```bash
brew install protobuf
```

**Linux:**
```bash
# Download from https://github.com/protocolbuffers/protobuf/releases
PROTOC_VERSION=25.1
wget https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip
unzip protoc-${PROTOC_VERSION}-linux-x86_64.zip -d $HOME/.local
export PATH=$PATH:$HOME/.local/bin
```

**Verify:**
```bash
protoc --version
# libprotoc 25.1
```

### 4. Go gRPC Plugins

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Verify they're in PATH
which protoc-gen-go
which protoc-gen-go-grpc
```

### 5. Confluent Cloud Account

1. Sign up at [confluent.cloud](https://confluent.cloud)
2. Create a Basic cluster (free tier works)
3. Create a topic: `payments.raw`
4. Generate API keys (we'll use them later)

---

## 🚀 Quick Start

### Clone & Setup

```bash
# Create project directory
mkdir payment-aggregator && cd payment-aggregator

# Initialize Go module
mkdir -p go-service && cd go-service
go mod init github.com/yourusername/payment-aggregator/go-service

# Install Go dependencies
go get google.golang.org/grpc
go get google.golang.org/protobuf
go get github.com/confluentinc/confluent-kafka-go/v2/kafka

cd ..

# Initialize Node.js project
mkdir -p node-service && cd node-service
npm init -y
npm install @grpc/grpc-js @grpc/proto-loader typescript ts-node @types/node
npx tsc --init
```

### Project Structure After Setup

```
payment-aggregator/
├── proto/
│   └── payment.proto
├── go-service/
│   ├── go.mod
│   ├── go.sum
│   └── cmd/
│       ├── producer/
│       └── server/
├── node-service/
│   ├── package.json
│   ├── tsconfig.json
│   └── src/
└── README.md
```

---

## ⚙️ Kafka Configuration

Create `go-service/.env` (DO NOT commit this file):

```env
# Confluent Cloud Config
KAFKA_BOOTSTRAP_SERVERS=<your-cluster>.confluent.cloud:9092
KAFKA_API_KEY=<your-api-key>
KAFKA_API_SECRET=<your-api-secret>
KAFKA_TOPIC=payments.raw

# gRPC Config
GRPC_PORT=50051
```

---

## 📝 Commands Reference

### Generate Protobuf Code

```bash
# From project root
protoc --go_out=go-service/pb --go_opt=paths=source_relative \
       --go-grpc_out=go-service/pb --go-grpc_opt=paths=source_relative \
       proto/payment.proto
```

### Run Go Producer (generate test data)

```bash
cd go-service
go run cmd/producer/main.go
```

### Run Go Server (consumer + gRPC)

```bash
cd go-service
go run cmd/server/main.go
```

### Run Node.js Client

```bash
cd node-service
npx ts-node src/index.ts
```

---

## 🧪 Testing the Pipeline

```
Terminal 1: Start Go Server
$ cd go-service && go run cmd/server/main.go
[SERVER] gRPC listening on :50051
[KAFKA] Connected to Confluent Cloud
[KAFKA] Consuming from payments.raw...

Terminal 2: Run Producer
$ cd go-service && go run cmd/producer/main.go
[PRODUCER] Sent: pay_abc123 | stripe | $50.00
[PRODUCER] Sent: pay_def456 | razorpay | ₹1000.00
...

Terminal 3: Run Node.js Client
$ cd node-service && npx ts-node src/index.ts
[CLIENT] Connected to gRPC server
[LOG] Payment: pay_abc123 | $50.00 USD | STRIPE | COMPLETED
[LOG] Payment: pay_def456 | ₹1000.00 INR | RAZORPAY | COMPLETED
```

---

## 📖 Resources

- [Go Documentation](https://go.dev/doc/)
- [gRPC Go Quick Start](https://grpc.io/docs/languages/go/quickstart/)
- [Protocol Buffers Guide](https://protobuf.dev/programming-guides/proto3/)
- [Confluent Kafka Go Client](https://github.com/confluentinc/confluent-kafka-go)

---

## 🤔 FAQ

**Q: Do I need Docker?**  
A: No, we're using Confluent Cloud for Kafka. Everything else runs locally.

**Q: Can I use a local Kafka instead?**  
A: Yes! Just update the bootstrap servers to `localhost:9092` and remove SASL config.

**Q: Why Go for the server and Node.js for client?**  
A: To demonstrate gRPC's polyglot nature. In production, use whatever fits your team.

---

## 📁 File Index

After completing all lessons, you'll have:

```
payment-aggregator/
├── README.md
├── proto/
│   └── payment.proto
├── go-service/
│   ├── .env
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   ├── producer/main.go
│   │   └── server/main.go
│   ├── internal/
│   │   ├── kafka/
│   │   │   ├── producer.go
│   │   │   └── consumer.go
│   │   ├── normalizer/normalizer.go
│   │   └── grpc/server.go
│   └── pb/
│       ├── payment.pb.go
│       └── payment_grpc.pb.go
├── node-service/
│   ├── package.json
│   ├── tsconfig.json
│   └── src/
│       ├── index.ts
│       └── client.ts
└── lessons/
    ├── 01-architecture.md
    ├── 02-go-basics.md
    └── ...
```