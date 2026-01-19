.PHONY: proto go-deps node-deps server producer dlq-test multi-topic client client-payments client-orders client-users deps setup clean

# Proto generation
proto:
	protoc --go_out=go-service/pb --go_opt=paths=source_relative \
	       --go-grpc_out=go-service/pb --go-grpc_opt=paths=source_relative \
	       -I go-service/proto \
	       go-service/proto/payment.proto
	cp go-service/proto/payment.proto node-service/proto/

# Install Go dependencies
go-deps:
	cd go-service && go mod tidy

# Install Node dependencies
node-deps:
	cd node-service && npm install

# Start Go server (consumers created on-demand)
server:
	cd go-service && go run cmd/server/main.go

# Run producer (single topic)
producer:
	cd go-service && go run cmd/producer/main.go -count=10 -interval=1s

# Run producer with invalid messages (DLQ test)
dlq-test:
	cd go-service && go run cmd/producer/main.go -count=2 -invalid=3 -interval=500ms

# Multi-topic load test: 100 valid + 10 invalid per topic, no delay
multi-topic:
	cd go-service && go run cmd/producer/main.go \
		-topics="poc.kafka.go.side.car.payments,poc.kafka.go.side.car.orders,poc.kafka.go.side.car.users" \
		-count=100 -invalid=10 -interval=0

# Start Node.js client (default topic)
client:
	cd node-service && npm run dev

# Start Node.js clients for specific topics
client-payments:
	cd node-service && npm run dev -- poc.kafka.go.side.car.payments

client-orders:
	cd node-service && npm run dev -- poc.kafka.go.side.car.orders

client-users:
	cd node-service && npm run dev -- poc.kafka.go.side.car.users

# Install all dependencies
deps: go-deps node-deps

# Full setup
setup: proto deps

# Clean generated files
clean:
	rm -f go-service/pb/*.go
	rm -rf node-service/dist
	rm -rf node-service/node_modules
