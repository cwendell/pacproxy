package config

import (
	"time"

	"github.com/IBM/sarama"
)

const (
	MessageTypeDelete string = "delete"
	MessageTypeStats  string = "stats"
)

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

func GetDefaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "statistics",
	}
}

func GetSaramaConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForLocal
	config.Producer.Return.Successes = false
	config.Producer.Return.Errors = false
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategySticky()}
	return config
}
