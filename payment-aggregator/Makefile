.PHONY: proto go-deps node-deps server producer dlq-test client all clean

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

# Start Go server
server:
	cd go-service && go run cmd/server/main.go

# Run producer
producer:
	cd go-service && go run cmd/producer/main.go -count=10 -interval=1s

# Run producer with invalid messages (DLQ test)
dlq-test:
	cd go-service && go run cmd/producer/main.go -count=2 -invalid=3 -interval=500ms

# Start Node.js client
client:
	cd node-service && npm run dev

# Install all dependencies
deps: go-deps node-deps

# Full setup
setup: proto deps

# Clean generated files
clean:
	rm -f go-service/pb/*.go
	rm -rf node-service/dist
	rm -rf node-service/node_modules