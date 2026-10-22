package main

import (
	"fmt"
	"log"
	"strings"
)

type KafkaPublisher struct {
	brokers   []string
	topic     string
	clusterID string
}

func NewKafkaPublisher(brokers []string, topic string, clusterID string) *KafkaPublisher {
	if clusterID == "" {
		clusterID = "cluster-primary"
	}
	return &KafkaPublisher{
		brokers:   brokers,
		topic:     topic,
		clusterID: clusterID,
	}
}

// FormatPartitionKey generates the tenant/cluster-isolated partition key: cluster_id + "/" + service_id
func (p *KafkaPublisher) FormatPartitionKey(serviceID string) string {
	cid := strings.TrimSpace(p.clusterID)
	if cid == "" {
		cid = "cluster-primary"
	}
	return fmt.Sprintf("%s/%s", cid, strings.TrimSpace(serviceID))
}

func (p *KafkaPublisher) Publish(data []byte) error {
	return p.PublishWithKey("", data)
}

func (p *KafkaPublisher) PublishWithKey(key string, data []byte) error {
	if key == "" {
		key = fmt.Sprintf("%s/global", p.clusterID)
	}
	log.Printf("[kafka-publisher] Publishing %d bytes to topic %s [partition_key: %s]", len(data), p.topic, key)
	return nil
}

