package mongoutils

import (
	"context"
	"pacproxy/shared/statistics"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	dbName            string = "gator"
	offsetsCollection string = "offsets"
	statsCollection   string = "statistics"
	mongoUri          string = "mongodb://localhost:27017"
	//mongoUri string = "mongodb+srv://mongodb5ix9t:r2N6U5xFzKaX1XO0@cluster0.tfkfn.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"
)

type PartitionOffset struct {
	Partition int32 `json:"partition" bson:"partition"`
	Offset    int64 `json:"offset" bson:"offset"`
}

// Opens a new MongoDB session. This function will attempt to connect to the database until
// its timeout is up
func InitMongoSession() (*mongo.Client, error) {
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(mongoUri).SetServerAPIOptions(serverAPI).SetTimeout(60 * time.Second)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// GetOffsets() queries and the last-written offsets for all specified Kafka partitions
func GetOffsets(client *mongo.Client, partitions []int32) (map[int32]int64, error) {
	collection := client.Database(dbName).Collection(offsetsCollection)
	filter := bson.D{{"partition", bson.D{{"$in", partitions}}}}
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	var partitionOffsets []PartitionOffset
	err = cursor.All(context.Background(), &partitionOffsets)
	if err != nil {
		return nil, err
	}
	offsetMap := make(map[int32]int64)
	offsetSliceToMap(partitionOffsets, offsetMap)
	return offsetMap, nil
}

// CreateUpdateOffsets() updates records of the last-written offset of each Kafka partition,
// or creates those records if they don't yet exist.
func CreateUpdateOffsets(client *mongo.Client, offsetMap map[int32]int64) (bool, error) {
	allAcknowledged := true
	collection := client.Database(dbName).Collection(offsetsCollection)
	var missedOffsets []*PartitionOffset
	var filter bson.D
	for part, offset := range offsetMap {
		filter = bson.D{{"partition", part}}
		partitionOffset := PartitionOffset{Partition: part, Offset: offset}
		replaceResult, err := collection.ReplaceOne(context.Background(), filter, partitionOffset)
		if err != nil {
			return false, err
		}
		if replaceResult.MatchedCount == 0 {
			missedOffsets = append(missedOffsets, &partitionOffset)
		}
		// Insert all partition offsets that weren't previously inserted
		if len(missedOffsets) > 0 {
			insertResult, err := collection.InsertMany(context.Background(), missedOffsets)
			if err != nil {
				return false, err
			}
			if !insertResult.Acknowledged {
				allAcknowledged = false
			}
		}
	}
	return allAcknowledged, nil
}

// GetAggregatedKeyStats() queries stats from all keystems and aggregates them into a
// single statistical summary. This approach is far from ideal, and a better solution is
// discussed in the README.
func GetAggregatedKeyStats(client *mongo.Client) (*statistics.AggregatedKeyStats, error) {
	collection := client.Database(dbName).Collection(statsCollection)
	filter := bson.D{{}}
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	var allKeyStats []statistics.KeyStats
	err = cursor.All(context.Background(), &allKeyStats)
	if err != nil {
		return nil, err
	}
	akStats := statistics.NewAggregatedKeyStats()
	for _, keyStats := range allKeyStats {
		akStats.AggregateKeyStats(&keyStats)
	}
	return akStats, nil
}

// GetKeyStats() queries and returns statistical summaries for each specified keystem.
func GetKeyStats(client *mongo.Client, keyStems []string) (map[string]*statistics.KeyStats, error) {
	collection := client.Database(dbName).Collection(statsCollection)
	filter := bson.D{{"usedKeystem", bson.D{{"$in", keyStems}}}}
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	var allKeyStats []statistics.KeyStats
	err = cursor.All(context.Background(), &allKeyStats)
	if err != nil {
		return nil, err
	}
	keystatsMap := make(map[string]*statistics.KeyStats)
	for _, keyStats := range allKeyStats {
		keystatsMap[keyStats.UsedKeystem] = &keyStats
	}
	return keystatsMap, nil
}

// CreateUpdateKeyStats() overwrites statistical summaries for each keystem, or creates
// records of these summaries if they don't already exist.
func CreateUpdateKeyStats(client *mongo.Client, keyStatsMap map[string]*statistics.KeyStats) (bool, error) {
	allAcknowledged := true
	collection := client.Database(dbName).Collection(statsCollection)
	var missedKeyStats []*statistics.KeyStats
	var filter bson.D
	for keyStem, keyStats := range keyStatsMap {
		filter = bson.D{{"usedKeystem", keyStem}}
		replaceResult, err := collection.ReplaceOne(context.Background(), filter, *keyStats)
		if err != nil {
			return false, err
		}
		if replaceResult.MatchedCount == 0 {
			missedKeyStats = append(missedKeyStats, keyStats)
		}

		// Insert stats that weren't replaced because they didn't exist
		if len(missedKeyStats) > 0 {
			insertResult, err := collection.InsertMany(context.Background(), missedKeyStats)
			if err != nil {
				return false, err
			}
			if !insertResult.Acknowledged {
				allAcknowledged = false
			}
		}
	}
	return allAcknowledged, nil
}

func offsetSliceToMap(offsets []PartitionOffset, offsetMap map[int32]int64) {
	for _, offset := range offsets {
		offsetMap[offset.Partition] = offset.Offset
	}
}
