package statistics

//go:generate easytags $GOFILE json:camel bson:camel

type DeleteRequest struct {
	UsedKeystem string `json:"usedKeystem" bson:"usedKeystem"`
}

// Statistics for a single request
type Stats struct {
	// Pack Request Stats
	UsedKeystem       string         `json:"usedKeystem" bson:"usedKeystem"`
	TotalItems        int            `json:"totalItems" bson:"totalItems"`
	TotalVolume       float64        `json:"totalVolume" bson:"totalVolume"`
	VolumeUtilization float64        `json:"volumeUtilization" bson:"volumeUtilization"`
	BoxTypes          map[string]int `json:"boxTypes" bson:"boxTypes"`

	// API Stats
	TimeStamp     string  `json:"timeStamp" bson:"timeStamp"`
	CacheHit      bool    `json:"cacheHit" bson:"cacheHit"`
	RequestError  bool    `json:"requestError" bson:"requestError"`
	ErrorResponse bool    `json:"errorResponse" bson:"errorResponse"`
	StatusCode    string  `json:"statusCode" bson:"statusCode"`
	Latency       float64 `json:"latency" bson:"latency"`
}

// Aggregated statistics for one keystem
type KeyStats struct {
	UsedKeystem          string         `json:"usedKeystem" bson:"usedKeystem"`
	TotalItems           int            `json:"totalItems" bson:"totalItems"`
	TotalVolume          float64        `json:"totalVolume" bson:"totalVolume"`
	AvgItemsPerPack      float64        `json:"avgItemsPerPack" bson:"avgItemsPerPack"`
	AvgVolumeUtilization float64        `json:"avgVolumeUtilization" bson:"avgVolumeUtilization"`
	BoxTypes             map[string]int `json:"boxTypes" bson:"boxTypes"`

	TotalRequests      int            `json:"totalRequests" bson:"totalRequests"`
	StatusCodes        map[string]int `json:"statusCodes" bson:"statusCodes"`
	RequestErrorCount  int            `json:"requestErrorCount" bson:"requestErrorCount"`
	ErrorResponseCount int            `json:"errorCount" bson:"errorResponseCount"`
	CacheHits          int            `json:"cacheHits" bson:"cacheHits"`
	HighestLatency     MaxLatency     `json:"maxLatency" bson:"highestLatency"`

	AvgLatency float64 `json:"avgLatency" bson:"avgLatency"`
}

type MaxLatency struct {
	Latency   float64 `json:"latency" bson:"latency"`
	TimeStamp string  `json:"timeStamp" bson:"timeStamp"`
}

// Aggregated statistics across multiple keystems
type AggregatedKeyStats struct {
	TotalItems           int            `json:"totalItems" bson:"totalItems"`
	TotalVolume          float64        `json:"totalVolume" bson:"totalVolume"`
	AvgItemsPerPack      float64        `json:"avgItemsPerPack" bson:"avgItemsPerPack"`
	AvgVolumeUtilization float64        `json:"avgVolumeUtilization" bson:"avgVolumeUtilization"`
	BoxTypes             map[string]int `json:"boxTypes" bson:"boxTypes"`

	TotalRequests      int            `json:"totalRequests" bson:"totalRequests"`
	StatusCodes        map[string]int `json:"statusCodes" bson:"statusCodes"`
	RequestErrorCount  int            `json:"requestErrorCount" bson:"requestErrorCount"`
	ErrorResponseCount int            `json:"errorCount" bson:"errorResponseCount"`
	CacheHits          int            `json:"cacheHits" bson:"cacheHits"`
	MaxLatency         struct {
		Latency     float64 `json:"latency" bson:"latency"`
		UsedKeystem string  `json:"usedKeystem" bson:"usedKeystem"`
		TimeStamp   string  `json:"timeStamp" bson:"timeStamp"`
	} `json:"maxLatency" bson:"maxLatency"`

	AvgLatency        float64 `json:"avgLatency" bson:"avgLatency"`
	HighestAvgLatency struct {
		Latency     float64 `json:"latency" bson:"latency"`
		UsedKeystem string  `json:"usedKeystem" bson:"usedKeystem"`
	} `json:"highestAvgLatency" bson:"highestAvgLatency"`
}

// New Stats instance with default values
func NewStats() *Stats {
	var stats Stats = Stats{
		BoxTypes: make(map[string]int),
	}
	return &stats
}

// New KeyStats instance with default values
func NewKeyStats(keystem string) *KeyStats {
	var keyStats KeyStats = KeyStats{}
	keyStats.Init(keystem)
	return &keyStats
}

// New AggregatedKeyStats with default values
func NewAggregatedKeyStats() *AggregatedKeyStats {
	akstats := AggregatedKeyStats{
		BoxTypes:    make(map[string]int),
		StatusCodes: make(map[string]int),
	}
	return &akstats
}

// Initialize values of a KeyStats instance
func (keyStats *KeyStats) Init(usedKeystem string) {
	keyStats.UsedKeystem = usedKeystem
	keyStats.TotalItems = 0
	keyStats.TotalVolume = 0
	keyStats.AvgItemsPerPack = 0
	keyStats.AvgVolumeUtilization = 0
	keyStats.BoxTypes = make(map[string]int)

	keyStats.TotalRequests = 0
	keyStats.StatusCodes = make(map[string]int)
	keyStats.RequestErrorCount = 0
	keyStats.ErrorResponseCount = 0
	keyStats.CacheHits = 0
	keyStats.HighestLatency = MaxLatency{
		Latency:   0,
		TimeStamp: "",
	}
	keyStats.AvgLatency = 0
}

func (keyStats *KeyStats) Reset() {
	keyStats.Init(keyStats.UsedKeystem)
}

// Aggregate the values from a Stats instance into a KeyStats instance
func (keyStats *KeyStats) AggregateStats(stats *Stats) {
	// Aggregate pack statistics
	keyStats.TotalItems = keyStats.TotalItems + stats.TotalItems
	keyStats.TotalVolume += stats.TotalVolume
	keyStats.AvgItemsPerPack = (keyStats.AvgItemsPerPack*float64(keyStats.TotalRequests) + float64(stats.TotalItems)) / (float64(keyStats.TotalRequests) + 1)
	keyStats.AvgVolumeUtilization = (keyStats.AvgVolumeUtilization*float64(keyStats.TotalRequests) + float64(stats.VolumeUtilization)) / (float64(keyStats.TotalRequests) + 1)
	for k, v := range stats.BoxTypes {
		_, keyExists := keyStats.BoxTypes[k]
		if keyExists {
			keyStats.BoxTypes[k] += v
		} else {
			keyStats.BoxTypes[k] = v
		}
	}
	// Aggregate API statistics
	if stats.StatusCode != "" {
		if _, keyExists := keyStats.StatusCodes[stats.StatusCode]; keyExists {
			keyStats.StatusCodes[stats.StatusCode]++
		} else {
			keyStats.StatusCodes[stats.StatusCode] = 1
		}
	}
	keyStats.RequestErrorCount += btoi(stats.RequestError)
	keyStats.ErrorResponseCount += btoi(stats.ErrorResponse)
	if !stats.CacheHit { // If cache hit, ignore the latency
		if stats.Latency > keyStats.HighestLatency.Latency {
			keyStats.HighestLatency.Latency = stats.Latency
			keyStats.HighestLatency.TimeStamp = stats.TimeStamp
		}
		keyStats.AvgLatency = ((keyStats.AvgLatency*float64(keyStats.TotalRequests-keyStats.CacheHits) + stats.Latency) / float64(keyStats.TotalRequests-keyStats.CacheHits+1))
	}
	keyStats.CacheHits += btoi(stats.CacheHit)
	keyStats.TotalRequests++
}

func (keyStats *KeyStats) AggregateKeyStats(other *KeyStats) {
	// Aggregate pack statistics
	keyStats.TotalItems += other.TotalItems
	keyStats.TotalVolume += other.TotalVolume
	keyStats.AvgItemsPerPack = (keyStats.AvgItemsPerPack*float64(keyStats.TotalRequests) + float64(other.TotalRequests)*other.AvgItemsPerPack) / (float64(keyStats.TotalRequests + other.TotalRequests))
	keyStats.AvgVolumeUtilization = (keyStats.AvgVolumeUtilization*float64(keyStats.TotalRequests) + float64(other.TotalRequests)*other.AvgVolumeUtilization) / (float64(keyStats.TotalRequests + other.TotalRequests))
	importCounts(other.BoxTypes, keyStats.BoxTypes)

	// Aggregate API statistics
	importCounts(other.StatusCodes, keyStats.StatusCodes)
	keyStats.RequestErrorCount += other.RequestErrorCount
	keyStats.ErrorResponseCount += other.ErrorResponseCount
	keyStats.AvgLatency = ((keyStats.AvgLatency*float64(keyStats.TotalRequests-keyStats.CacheHits) + other.AvgItemsPerPack*float64(other.TotalRequests-other.CacheHits)) / float64(keyStats.TotalRequests-keyStats.CacheHits+other.TotalRequests-other.CacheHits))

	keyStats.CacheHits += other.CacheHits
	keyStats.TotalRequests += other.TotalRequests
}

func (akStats *AggregatedKeyStats) AggregateKeyStats(keyStats *KeyStats) {
	// Aggregate pack statistics
	akStats.TotalItems += keyStats.TotalItems
	akStats.TotalVolume += keyStats.TotalVolume
	akStats.AvgItemsPerPack = (akStats.AvgItemsPerPack*float64(akStats.TotalRequests) + float64(keyStats.TotalRequests)*keyStats.AvgItemsPerPack) / (float64(akStats.TotalRequests + keyStats.TotalRequests))
	akStats.AvgVolumeUtilization = (akStats.AvgVolumeUtilization*float64(akStats.TotalRequests) + float64(keyStats.TotalRequests)*keyStats.AvgVolumeUtilization) / (float64(akStats.TotalRequests + keyStats.TotalRequests))
	importCounts(keyStats.BoxTypes, akStats.BoxTypes)

	// Aggregate API statistics
	importCounts(keyStats.StatusCodes, akStats.StatusCodes)
	akStats.RequestErrorCount += keyStats.RequestErrorCount
	akStats.ErrorResponseCount += keyStats.ErrorResponseCount
	if keyStats.HighestLatency.Latency > akStats.MaxLatency.Latency {
		akStats.MaxLatency.Latency = keyStats.HighestLatency.Latency
		akStats.MaxLatency.UsedKeystem = keyStats.UsedKeystem
		akStats.MaxLatency.TimeStamp = keyStats.HighestLatency.TimeStamp
	}
	if keyStats.AvgLatency > akStats.HighestAvgLatency.Latency {
		akStats.HighestAvgLatency.Latency = keyStats.AvgLatency
		akStats.HighestAvgLatency.UsedKeystem = keyStats.UsedKeystem
	}
	akStats.AvgLatency = ((akStats.AvgLatency * float64(akStats.TotalRequests-akStats.CacheHits)) + (keyStats.AvgLatency * float64(keyStats.TotalRequests-keyStats.CacheHits))) / (float64(akStats.TotalRequests + keyStats.TotalRequests - akStats.CacheHits - keyStats.CacheHits))

	akStats.CacheHits += keyStats.CacheHits
	akStats.TotalRequests += keyStats.TotalRequests
}

func importCounts(source map[string]int, dest map[string]int) {
	for k, v := range source {
		_, keyExists := dest[k]
		if keyExists {
			dest[k] += v
		} else {
			dest[k] = v
		}
	}
}

// Encodes boolean as integer
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
