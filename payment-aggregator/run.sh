#!/bin/bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║          Payment Aggregator - gRPC Kafka Sidecar              ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""

case "$1" in
  setup)
    echo -e "${YELLOW}Setting up project...${NC}"
    make setup
    echo -e "${GREEN}Setup complete!${NC}"
    ;;
  server)
    echo -e "${YELLOW}Starting Go gRPC server...${NC}"
    cd go-service && go run cmd/server/main.go
    ;;
  client)
    TOPIC=${2:-poc.kafka.go.side.car}
    echo -e "${YELLOW}Starting Node.js client for topic: ${TOPIC}${NC}"
    cd node-service && npm run dev -- "$TOPIC"
    ;;
  producer)
    COUNT=${2:-5}
    INVALID=${3:-0}
    echo -e "${YELLOW}Sending ${COUNT} test payments...${NC}"
    if [ "$INVALID" -gt 0 ]; then
      echo -e "${YELLOW}Also sending ${INVALID} invalid messages for DLQ testing...${NC}"
    fi
    cd go-service && go run cmd/producer/main.go -count=$COUNT -invalid=$INVALID -interval=1s
    ;;
  dlq-test)
    echo -e "${YELLOW}Testing DLQ: Sending 2 valid + 3 invalid messages...${NC}"
    cd go-service && go run cmd/producer/main.go -count=2 -invalid=3 -interval=500ms
    ;;
  multi-topic)
    TOPICS="poc.kafka.go.side.car.payments,poc.kafka.go.side.car.orders,poc.kafka.go.side.car.users"
    COUNT=${2:-100}
    INVALID=${3:-10}
    echo -e "${YELLOW}Multi-topic test: ${COUNT} valid + ${INVALID} invalid per topic${NC}"
    echo -e "${YELLOW}Topics: ${TOPICS}${NC}"
    cd go-service && go run cmd/producer/main.go -topics="$TOPICS" -count=$COUNT -invalid=$INVALID -interval=0
    ;;
  *)
    echo "Usage: $0 {setup|server|client|producer|multi-topic|dlq-test}"
    echo ""
    echo "Commands:"
    echo "  setup                  - Install dependencies and generate proto"
    echo "  server                 - Start Go gRPC server (consumers created on-demand)"
    echo "  client [topic]         - Start Node.js client for a topic"
    echo "  producer [n] [i]       - Send n valid + i invalid payments"
    echo "  multi-topic [n] [i]    - Send to 3 topics: n valid + i invalid each (no delay)"
    echo "  dlq-test               - Quick DLQ test (2 valid + 3 invalid)"
    echo ""
    echo "Quick start (run in separate terminals):"
    echo "  Terminal 1: ./run.sh server"
    echo "  Terminal 2: ./run.sh client poc.kafka.go.side.car"
    echo "  Terminal 3: ./run.sh producer 10"
    echo ""
    echo "Multi-topic load test (3 topics, 100 msgs each + DLQ):"
    echo "  Terminal 1: ./run.sh server"
    echo "  Terminal 2: ./run.sh client poc.kafka.go.side.car.payments"
    echo "  Terminal 3: ./run.sh client poc.kafka.go.side.car.orders"
    echo "  Terminal 4: ./run.sh client poc.kafka.go.side.car.users"
    echo "  Terminal 5: ./run.sh multi-topic 100 10"
    echo ""
    echo "DLQ testing:"
    echo "  ./run.sh dlq-test              # Quick test"
    echo "  ./run.sh producer 5 3          # 5 valid + 3 invalid"
    ;;
esac
