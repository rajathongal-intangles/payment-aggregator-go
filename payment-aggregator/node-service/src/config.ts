import dotenv from 'dotenv';
import path from 'path';

dotenv.config();

export const config = {
  grpc: {
    host: process.env.GRPC_SERVER_HOST || 'localhost',
    port: process.env.GRPC_SERVER_PORT || '50051',
    get address() {
      return `${this.host}:${this.port}`;
    },
  },
  kafka: {
    topic: process.env.KAFKA_TOPIC || 'poc.kafka.go.side.car',
  },
  proto: {
    path: path.join(__dirname, '../proto/payment.proto'),
  },
  logLevel: process.env.LOG_LEVEL || 'info',
};