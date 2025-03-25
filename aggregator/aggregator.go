package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"pacproxy/shared/config"
	"pacproxy/shared/mongoutils"
	"pacproxy/shared/statistics"
	"sync"
	"time"

	"log/slog"

	"github.com/IBM/sarama"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

const (
	//mongoUri string = "localhost"
	dbWriteInterval time.Duration = 5 * time.Second
	consumerGroupId string        = "aggregators"
)

type consumerHandler struct {
	sessionGenerationId int32
	mongoClient         *mongo.Client
	statsByKeystem      map[string]*statistics.KeyStats
	offsetByPartition   map[int32]int64
	consumerGroup       *sarama.ConsumerGroup
	lastWrite           time.Time
}

func main() {

	slog.SetLogLoggerLevel(slog.LevelDebug)
	var handler consumerHandler
	var err error

	// Attempt to connect to mongodb
	handler.statsByKeystem = make(map[string]*statistics.KeyStats)
	handler.offsetByPartition = make(map[int32]int64)
	handler.mongoClient, err = mongoutils.InitMongoSession()
	if err != nil {
		slog.Error(fmt.Errorf("couldn't initiate a mongodb connection: %s", err).Error())
		return
	}
	defer func() {
		if err = handler.mongoClient.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()
	slog.Info("connected to mongodb")

	// Attempt to connect to kafka
	kafkaConfig := config.GetDefaultKafkaConfig()
	saramaConfig := config.GetSaramaConfig()
	client, err := sarama.NewConsumerGroup(kafkaConfig.Brokers, consumerGroupId, saramaConfig)
	if err != nil {
		slog.Error(fmt.Errorf("couldn't initiate a connection to kafka: %s", err).Error())
		return
	}
	defer client.Close()

	slog.Info("connected to kafka")
	handler.consumerGroup = &client

	// Start consumer process
	slog.Info("starting the consumer")
	ctx, cancel := context.WithCancel(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			slog.Debug("consuming")
			err := client.Consume(ctx, []string{kafkaConfig.Topic}, &handler)
			if err != nil {
				slog.Error(fmt.Errorf("Error consuming from queue: %s", err).Error())
			}
			select {
			case <-signals:
				cancel()
			default:
			}
		}
	}()

	wg.Wait()
}

// Setup() runs when the consumer is initializing or is reconnecting to kafka
func (h *consumerHandler) Setup(sess sarama.ConsumerGroupSession) error {
	slog.Debug("Setting up handler")
	h.loadReset(sess)
	return nil
}

// Cleanup() runs before a consumer is closed, allowing final offset commits
func (h *consumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim() is called when consuming messages from a topic
func (h *consumerHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	// If partition assignment have been rebalanced, pause messages retrieval and get new offsets from the database
	if h.sessionGenerationId != sess.GenerationID() {
		slog.Info("Partition reassignment detected, fetching stored data.")
		(*h.consumerGroup).PauseAll()
		h.loadReset(sess)
		(*h.consumerGroup).ResumeAll()
	}

	// Process messages
	for msg := range claim.Messages() {
		messageType := getMessageType(msg)
		slog.Debug("message received: type: " + messageType)
		// If we see a delete request, we reset all data for that keystem and write it to the DB
		if messageType == config.MessageTypeDelete {
			var dr statistics.DeleteRequest
			err := json.Unmarshal(msg.Value, &dr)
			if err != nil {
				slog.Error(fmt.Errorf("error occurred while unmarshaling delete request:%s", err).Error())
				continue
			}
			if _, keyExists := h.statsByKeystem[dr.UsedKeystem]; !keyExists {
				// If the keystem being deleted isn't stored locally, create an empty one to write
				h.statsByKeystem[dr.UsedKeystem] = statistics.NewKeyStats(dr.UsedKeystem)
			}
			h.statsByKeystem[dr.UsedKeystem].Reset()
			h.offsetByPartition[msg.Partition] = msg.Offset
			h.writeStats()
			// If we see a stats message, we aggregate it with the rest
		} else if messageType == config.MessageTypeStats {
			var stats statistics.Stats
			err := json.Unmarshal(msg.Value, &stats)
			if err != nil {
				slog.Error(fmt.Errorf("error occurred while unmarshaling stats message:%s", err).Error())
				continue
			}
			if _, keyExists := h.statsByKeystem[stats.UsedKeystem]; !keyExists {
				h.statsByKeystem[stats.UsedKeystem] = statistics.NewKeyStats(stats.UsedKeystem) // initialize in case the database does not have any data on this keystem
				keyStatsMap, err := mongoutils.GetKeyStats(h.mongoClient, []string{stats.UsedKeystem})

				if err != nil {
					slog.Error(fmt.Errorf("error occurred while getting stats from the database: %s", err).Error())
				}
				for keystem, keyStats := range keyStatsMap {
					h.statsByKeystem[keystem] = keyStats
				}
			}
			h.statsByKeystem[stats.UsedKeystem].AggregateStats(&stats)
			h.offsetByPartition[msg.Partition] = msg.Offset // Update offset
			// Write stats to db if enough time has passed since last write
			log.Println(time.Since(h.lastWrite))
			if time.Since(h.lastWrite) >= dbWriteInterval {
				slog.Debug("Writing stats")
				err := h.writeStats()
				if err != nil {
					slog.Error(fmt.Errorf("write aborted: error occurred while writing stats to the database: %s", err).Error())
				} else {
					h.lastWrite = time.Now()
					slog.Debug("stats written successfully")
				}
			}
		} else {
			slog.Debug("No message header")
		}
		sess.MarkMessage(msg, "")
	}

	return nil
}

// writeStats() writes keystats and the updated partition offset to the database
func (h *consumerHandler) writeStats() error {
	wc := writeconcern.Majority()
	txnOptions := options.Transaction().SetWriteConcern(wc)
	// Starts a session on the client
	session, err := h.mongoClient.StartSession()
	if err != nil {
		return err
	}
	// Defers ending the session after the transaction is committed or ended
	defer session.EndSession(context.TODO())
	// Inserts multiple documents into a collection within a transaction,
	// then commits or ends the transaction
	_, err = session.WithTransaction(context.TODO(), func(ctx context.Context) (interface{}, error) {
		offsetAck, err := mongoutils.CreateUpdateOffsets(h.mongoClient, h.offsetByPartition)
		if err != nil {
			return offsetAck, err
		} else if !offsetAck {
			return offsetAck, fmt.Errorf("write acknowledgement not received from database")
		}
		statsAck, err := mongoutils.CreateUpdateKeyStats(h.mongoClient, h.statsByKeystem)
		if err != nil {
			return statsAck, err
		} else if !statsAck {
			return statsAck, fmt.Errorf("write acknowledgement not received from database")
		}
		return true, nil
	}, txnOptions)
	return err
}

// LoadReset() loads and applies the new partition offsets from the database and clears previous KeyStats
// Called after startup or a partition reassignment.
func (h *consumerHandler) loadReset(sess sarama.ConsumerGroupSession) error {
	obp, err := mongoutils.GetOffsets(h.mongoClient, sess.Claims()[config.GetDefaultKafkaConfig().Topic])
	h.offsetByPartition = obp
	if err != nil {
		return err
	}
	// Set new offsets for each partition if an offset for that partition one was saved to the database
	for part, offset := range h.offsetByPartition {
		sess.ResetOffset(config.GetDefaultKafkaConfig().Topic, part, offset, "")
	}
	// delete all KeyStats
	h.clearKeyStats()
	return nil
}

// clearKeyStats() deletes all keys in our map of KeyStats. This is called when partition
// assignment are updated, and not when a request comes in to delete keystem data.
func (h *consumerHandler) clearKeyStats() {
	for k := range h.statsByKeystem {
		delete(h.statsByKeystem, k)
	}
}

// getMessageType() returns a string representing the type of message (stats or delete)
func getMessageType(msg *sarama.ConsumerMessage) string {
	for _, recordHeader := range msg.Headers {
		if string(recordHeader.Key) == "type" {
			return string(recordHeader.Value)
		}
	}
	return ""
}
