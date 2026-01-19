package config

import (
	"os"
)

type Config struct {
	// Kafka
	KafkaBrokers       string
	KafkaSSL           bool
	KafkaSASLMechanism string
	KafkaSASLUsername  string
	KafkaSASLPassword  string
	KafkaTopic         string
	KafkaGroupID       string

	// Schema Registry (optional)
	SchemaRegistryURL  string
	SchemaRegistryAuth string

	// gRPC
	GRPCPort string
}

func Load() *Config {
	return &Config{
		KafkaBrokers:       getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaSSL:           getEnv("KAFKA_SSL", "false") == "true",
		KafkaSASLMechanism: getEnv("KAFKA_SASL_MECHANISM", "PLAIN"),
		KafkaSASLUsername:  getEnv("KAFKA_SASL_USERNAME", ""),
		KafkaSASLPassword:  getEnv("KAFKA_SASL_PASSWORD", ""),
		KafkaTopic:         getEnv("KAFKA_TOPIC", "poc.kafka.go.side.car"),
		KafkaGroupID:       getEnv("KAFKA_GROUP_ID", "payment-service"),
		SchemaRegistryURL:  getEnv("KAFKA_SCHEMA_REGISTRY_URL", ""),
		SchemaRegistryAuth: getEnv("KAFKA_SCHEMA_REGISTRY_AUTH", ""),
		GRPCPort:           getEnv("GRPC_PORT", "50051"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// HasSASL checks if SASL authentication is configured
func (c *Config) HasSASL() bool {
	return c.KafkaSASLUsername != "" && c.KafkaSASLPassword != ""
}