#!/bin/bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║          Payment Aggregator - Quick Start                     ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""

case "$1" in
  setup)
    echo -e "${YELLOW}Setting up project...${NC}"
    make setup
    echo -e "${GREEN}✅ Setup complete!${NC}"
    ;;
  server)
    echo -e "${YELLOW}Starting Go gRPC server...${NC}"
    cd go-service && go run cmd/server/main.go
    ;;
  client)
    echo -e "${YELLOW}Starting Node.js client...${NC}"
    cd node-service && npm run dev
    ;;
  producer)
    COUNT=${2:-5}
    echo -e "${YELLOW}Sending ${COUNT} test payments...${NC}"
    cd go-service && go run cmd/producer/main.go -count=$COUNT -interval=1s
    ;;
  *)
    echo "Usage: $0 {setup|server|client|producer [count]}"
    echo ""
    echo "Commands:"
    echo "  setup     - Install dependencies and generate proto"
    echo "  server    - Start Go gRPC server"
    echo "  client    - Start Node.js streaming client"
    echo "  producer  - Send test payments (default: 5)"
    echo ""
    echo "Quick start (run in separate terminals):"
    echo "  Terminal 1: ./run.sh server"
    echo "  Terminal 2: ./run.sh client"
    echo "  Terminal 3: ./run.sh producer 10"
    ;;
esac