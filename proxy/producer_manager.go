package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"pacproxy/shared/config"
	"pacproxy/shared/statistics"
	"time"

	"github.com/IBM/sarama"
)

// initProducer() opens a connection with the Kafka producer
func initProducer(timeout time.Duration) (*sarama.AsyncProducer, error) {
	kafkaConfig := config.GetDefaultKafkaConfig()
	saramaConfig := config.GetSaramaConfig()

	producer, err := sarama.NewAsyncProducer(kafkaConfig.Brokers, saramaConfig)
	if err != nil {
		return nil, err
	}
	return &producer, nil
}

// sendMessage() sends a kafka message holding an encoded object
// producer: kafka producer
// v: object to be encoded
// messageType: the type of message (stats message or delete request)
func sendMessage(producer *sarama.AsyncProducer, v any, messageType string) error {

	var usedKeyStem string
	if messageType == config.MessageTypeStats {
		stats := v.(*statistics.Stats)
		usedKeyStem = stats.UsedKeystem
	} else if messageType == config.MessageTypeDelete {
		request := v.(*statistics.DeleteRequest)
		usedKeyStem = request.UsedKeystem
	}

	producerMessage, err := buildProducerMessage(v, hash(usedKeyStem), messageType)
	if err != nil {
		return fmt.Errorf("Error occurred while building message: %s", err)
	}
	(*producer).Input() <- producerMessage
	return nil
}

// buildProducerMessage() encodes a message from an object.
// v: object to be encoded
// partition: kafka partition the message will be written to
// messageType: the type of message (stats message or delete request)
func buildProducerMessage(v any, partition int32, messageType string) (*sarama.ProducerMessage, error) {
	config := config.GetDefaultKafkaConfig()
	messageBody, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	message := &sarama.ProducerMessage{
		Topic:     config.Topic,
		Value:     sarama.StringEncoder(messageBody),
		Partition: partition,
		Headers:   []sarama.RecordHeader{sarama.RecordHeader{Key: []byte("type"), Value: []byte(messageType)}},
	}
	return message, nil
}

// hash() returns a hash of a string.
// This is used to assign partition numbers based on keystem.
func hash(s string) int32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int32(h.Sum32())
}

// Todo write process to resend unacknowledged messages
