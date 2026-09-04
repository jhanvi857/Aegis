package main

import (
	"log"
)

type KafkaPublisher struct {
	brokers []string
	topic   string
}

func NewKafkaPublisher(brokers []string, topic string) *KafkaPublisher {
	return &KafkaPublisher{
		brokers: brokers,
		topic:   topic,
	}
}

func (p *KafkaPublisher) Publish(data []byte) error {
	// Phase 1 implementation will publish protobuf TelemetryBatch to Kafka
	log.Printf("[kafka-publisher] Stub publishing %d bytes to topic %s", len(data), p.topic)
	return nil
}
