package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"log/slog"
	"pacproxy/shared/config"
	"pacproxy/shared/mongoutils"
	"pacproxy/shared/statistics"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type PackRequest struct {
	RequestID string `json:"requestId" bson:"requestID"`
	OrderID   string `json:"orderId" bson:"orderID"`
	LayFlat   bool   `json:"layFlat" bson:"layFlat"`
	Interlock bool   `json:"interlock" bson:"interlock"`
	Corners   bool   `json:"corners" bson:"corners"`
	ItemSets  []struct {
		RefID      int    `json:"refId"`
		Color      string `json:"color"`
		Dimensions struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		} `json:"dimensions" validate:"required"`
		Weight   float64 `json:"weight" validate:"required"`
		Quantity int     `json:"quantity" validate:"required"`
	} `json:"itemSets" bson:"itemSets"`
	BoxTypes []struct {
		Name       string `json:"name"`
		Dimensions struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		} `json:"dimensions" validate:"required"`
		WeightMax  float64 `json:"weightMax" validate:"required"`
		WeightTare float64 `json:"weightTare"`
		Price      int     `json:"price"`
	} `json:"boxTypes" bson:"boxTypes"`
	BoxTypeGenerators []struct {
		BoxTypeDefaults struct {
			Name       string `json:"name"`
			RefID      int    `json:"refId"`
			Price      int    `json:"price"`
			WeightTare int    `json:"weightTare"`
			WeightMax  int    `json:"weightMax" validate:"required"`
			Dimensions struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"dimensions" validate:"required"`
			Outer struct {
				Dimensions struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"dimensions"`
				DimensionChange struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"dimensionChange"`
			} `json:"outer"`
			CenterOfMass struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"centerOfMass"`
			ReservedSpace     float64 `json:"reservedSpace"`
			ItemsPerBoxMax    int     `json:"itemsPerBoxMax"`
			ItemSetsPerBoxMax int     `json:"itemSetsPerBoxMax"`
			ItemsInlineMax    []int   `json:"itemsInlineMax"`
			RateTable         struct {
				Carrier           string `json:"carrier"`
				Service           string `json:"service"`
				Zone              string `json:"zone"`
				Rates             []int  `json:"rates"`
				Weights           []int  `json:"weights"`
				PriceIncreaseRate int    `json:"priceIncreaseRate"`
				BasePrice         int    `json:"basePrice"`
				DimFactor         int    `json:"dimFactor"`
			} `json:"rateTable"`
			PropertyConstraints []struct {
				Key       string `json:"key"`
				Max       int    `json:"max"`
				Aggregate string `json:"aggregate"`
				Value     int    `json:"value"`
			} `json:"propertyConstraints"`
		} `json:"boxTypeDefaults"`
		Operation string `json:"operation"`
		Options   struct {
			XList  []int `json:"xList"`
			XRange struct {
				Min             int  `json:"min"`
				Max             int  `json:"max"`
				DeriveFromItems bool `json:"deriveFromItems"`
				FitForFirstItem bool `json:"fitForFirstItem"`
				Increment       int  `json:"increment"`
			} `json:"xRange"`
			YList  []int `json:"yList"`
			YRange struct {
				Min             int  `json:"min"`
				Max             int  `json:"max"`
				DeriveFromItems bool `json:"deriveFromItems"`
				FitForFirstItem bool `json:"fitForFirstItem"`
				Increment       int  `json:"increment"`
			} `json:"yRange"`
			ZList  []int `json:"zList"`
			ZRange struct {
				Min             int  `json:"min"`
				Max             int  `json:"max"`
				DeriveFromItems bool `json:"deriveFromItems"`
				FitForFirstItem bool `json:"fitForFirstItem"`
				Increment       int  `json:"increment"`
			} `json:"zRange"`
			Limits []struct {
				Metric string `json:"metric"`
				Min    int    `json:"min"`
				Max    int    `json:"max"`
			} `json:"limits"`
			PriceComponents []struct {
				Key        string `json:"key"`
				Metric     string `json:"metric"`
				Aggregator string `json:"aggregator"`
				Thresholds []int  `json:"thresholds"`
				Prices     []int  `json:"prices"`
			} `json:"priceComponents"`
			NoTrimToMaxExtent string `json:"noTrimToMaxExtent"`
		} `json:"options"`
	} `json:"boxTypeGenerators" bson:"boxTypeGenerators"`
	BoxTypeDefaults struct {
		WeightMax int `json:"weightMax" validate:"required"`
		RateTable struct {
			DimFactor float64 `json:"dimFactor"`
		} `json:"rateTable"`
	} `json:"boxTypeDefaults" bson:"boxTypeDefaults"`
	Boxes []struct {
		WeightMax  float64 `json:"weightMax" validate:"required"`
		Dimensions struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		} `json:"dimensions" validate:"required"`
	} `json:"boxes" bson:"boxes"`
	UsableSpace   float64  `json:"usableSpace" bson:"usableSpace"`
	ReservedSpace float64  `json:"reservedSpace" bson:"reservedSpace"`
	BoxTypeSets   []string `json:"boxTypeSets" bson:"boxTypeSets"`
	Eye           struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"eye" bson:"eye"`
	PackOrigin struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"packOrigin" bson:"packOrigin"`
	Zone  int `json:"zone" bson:"zone"`
	Rules []struct {
		ItemRefId    int    `json:"itemRefId"`
		ItemSequence string `json:"itemSequence"`
		ItemMatch    struct {
			All         bool     `json:"all"`
			Property    string   `json:"property"`
			Expression  string   `json:"expression"`
			Expressions []string `json:"expressions"`
			Negate      bool     `json:"negate"`
		} `json:"itemMatch"`
		TargetItemRefIds    []int    `json:"targetItemRefIds"`
		TargetItemSequences []string `json:"targetItemSequences"`
		TargetBoxRefIds     []int    `json:"targetBoxRefIds"`
		Operation           string   `json:"operation" validate:"required"`
		Options             any      `json:"options"`
		Parameters          []string `json:"parameters"`
	} `json:"rules" bson:"rules"`
	Random                           bool    `json:"random" bson:"random"`
	N                                int     `json:"n" bson:"n"`
	RandomMaxDimension               int     `json:"randomMaxDimension" bson:"randomMaxDimension"`
	RandomMaxWeight                  int     `json:"randomMaxWeight" bson:"randomMaxWeight"`
	RandomMaxQuantity                int     `json:"randomMaxQuantity" bson:"randomMaxQuantity"`
	Seed                             bool    `json:"seed" bson:"seed"`
	SeedValue                        int     `json:"seedValue" bson:"seedValue"`
	ImgSize                          int     `json:"imgSize" bson:"imgSize"`
	Template                         string  `json:"template" bson:"template"`
	IncludeScripts                   bool    `json:"includeScripts" bson:"includeScripts"`
	IncludeImages                    bool    `json:"includeImages" bson:"includeImages"`
	ImageFormat                      string  `json:"imageFormat" bson:"imageFormat"`
	CoordOrder                       []int   `json:"coordOrder" bson:"coordOrder"`
	CohortPacking                    bool    `json:"cohortPacking" bson:"cohortPacking"`
	CohortMax                        int     `json:"cohortMax" bson:"cohortMax"`
	AllowableOverhang                float64 `json:"allowableOverhang" bson:"allowableOverhang"`
	PlacementStyle                   string  `json:"placementStyle" bson:"placementStyle"`
	ItemSort                         string  `json:"itemSort" bson:"itemSort"`
	ItemSortReverse                  bool    `json:"itemSortReverse" bson:"itemSortReverse"`
	ItemSortDualDirection            bool    `json:"itemSortDualDirection" bson:"itemSortDualDirection"`
	ItemInitialOrientationBestForBox bool    `json:"itemInitialOrientationBestForBox" bson:"itemInitialOrientationBestForBox"`
	ItemInitialOrientationPreferred  bool    `json:"itemInitialOrientationPreferred" bson:"itemInitialOrientationPreferred"`
	ItemOrientationSearchDepth       int     `json:"itemOrientationSearchDepth" bson:"itemOrientationSearchDepth"`
	SequenceSort                     bool    `json:"sequenceSort" bson:"sequenceSort"`
	SequenceHeatMap                  bool    `json:"sequenceHeatMap" bson:"sequenceHeatMap"`
	MaxSequenceDistance              int     `json:"maxSequenceDistance" bson:"maxSequenceDistance"`
	BoxTypeChoiceStyle               string  `json:"boxTypeChoiceStyle" bson:"boxTypeChoiceStyle"`
	BoxTypeChoiceLookahead           int     `json:"boxTypeChoiceLookahead" bson:"boxTypeChoiceLookahead"`
	BoxTypeChoiceLookback            int     `json:"boxTypeChoiceLookback" bson:"boxTypeChoiceLookback"`
	BoxTypeChoiceGoal                string  `json:"boxTypeChoiceGoal" bson:"boxTypeChoiceGoal"`
	BoxesMax                         int     `json:"boxesMax" bson:"boxesMax"`
	BoxesPerItemSetMax               int     `json:"boxesPerItemSetMax" bson:"boxesPerItemSetMax"`
	BoxesPerSequenceMax              int     `json:"boxesPerSequenceMax" bson:"boxesPerSequenceMax"`
	ItemsPerBoxMax                   int     `json:"itemsPerBoxMax" bson:"itemsPerBoxMax"`
	ItemSetsPerBoxMax                int     `json:"itemSetsPerBoxMax" bson:"itemSetsPerBoxMax"`
	ItemsInlineMax                   []int   `json:"itemsInlineMax" bson:"itemsInlineMax"`
	GeneratedBoxTypesMax             int     `json:"generatedBoxTypesMax" bson:"generatedBoxTypesMax"`
	ValueTiebreaker                  string  `json:"valueTiebreaker" bson:"valueTiebreaker"`
	Timeout                          float64 `json:"timeout" bson:"timeout"`
}

type PackResponse struct {
	BoxTypeChoiceGoalUsed string `json:"boxTypeChoiceGoalUsed" bson:"boxTypeChoiceGoalUsed"`
	Boxes                 []struct {
		Box struct {
			BoxType struct {
				CenterOfMass struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"centerOfMass"`
				Dimensions struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"dimensions"`
				ItemsInlineOverhang struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"itemsInlineOverhang"`
				Name      string `json:"name"`
				Price     int    `json:"price"`
				RateTable struct {
					BasePrice         float64   `json:"basePrice"`
					Carrier           string    `json:"carrier"`
					DimFactor         float64   `json:"dimFactor"`
					PriceIncreaseRate float64   `json:"priceIncreaseRate"`
					Rates             []float64 `json:"rates"`
					Service           string    `json:"service"`
					Weights           []float64 `json:"weights"`
					Zone              string    `json:"zone"`
				} `json:"rateTable"`
				RefID         int     `json:"refId"`
				ReservedSpace any     `json:"reservedSpace"`
				WeightMax     float64 `json:"weightMax"`
				WeightTare    float64 `json:"weightTare"`
			} `json:"boxType"`
			CenterOfMass struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"centerOfMass"`
			CenterOfMassString    string `json:"centerOfMassString"`
			DepthOrder            []int  `json:"depthOrder"`
			DepthOrderString      string `json:"depthOrderString"`
			DimensionalWeight     int    `json:"dimensionalWeight"`
			DimensionalWeightUsed bool   `json:"dimensionalWeightUsed"`
			Dimensions            struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"dimensions"`
			ID    int `json:"id"`
			Items []struct {
				Item struct {
					CenterOfMass struct {
						X float64 `json:"x"`
						Y float64 `json:"y"`
						Z float64 `json:"z"`
					} `json:"centerOfMass"`
					Color      string `json:"color"`
					DeltaCost  int    `json:"deltaCost"`
					Dimensions struct {
						X float64 `json:"x"`
						Y float64 `json:"y"`
						Z float64 `json:"z"`
					} `json:"dimensions"`
					GlobalID int    `json:"globalId"`
					Index    int    `json:"index"`
					Message  string `json:"message"`
					Name     string `json:"name"`
					Origin   struct {
						X float64 `json:"x"`
						Y float64 `json:"y"`
						Z float64 `json:"z"`
					} `json:"origin"`
					PackedIndex int     `json:"packedIndex"`
					Properties  any     `json:"properties"`
					Quantity    int     `json:"quantity"`
					RefID       int     `json:"refId"`
					Sequence    string  `json:"sequence"`
					UniqueID    string  `json:"uniqueId"`
					Virtual     bool    `json:"virtual"`
					Weight      float64 `json:"weight"`
				} `json:"item"`
			} `json:"items"`
			ItemsInlineMax    []int  `json:"itemsInlineMax"`
			ItemSetsPerBoxMax int    `json:"itemSetsPerBoxMax"`
			ItemsPerBoxMax    int    `json:"itemsPerBoxMax"`
			LenItems          int    `json:"lenItems"`
			LenUnits          int    `json:"lenUnits"`
			Name              string `json:"name"`
			Outer             struct {
				Dimensions struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"dimensions"`
				DimensionsChange struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					Z float64 `json:"z"`
				} `json:"dimensionsChange"`
			} `json:"outer"`

			Origin struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"origin"`
			Price               int `json:"price"`
			Properties          any `json:"properties"`
			PropertyConstraints any `json:"propertyConstraints"`
			RateTable           struct {
				BasePrice         float64   `json:"basePrice"`
				Carrier           string    `json:"carrier"`
				DimFactor         float64   `json:"dimFactor"`
				PriceIncreaseRate float64   `json:"priceIncreaseRate"`
				Rates             []float64 `json:"rates"`
				Service           string    `json:"service"`
				Weights           []float64 `json:"weights"`
				Zone              string    `json:"zone"`
			} `json:"rateTable"`
			RefID             int     `json:"refId"`
			ReservedSpace     float64 `json:"reservedSpace"`
			Subspace          any     `json:"subspace"`
			VolumeMax         int     `json:"volumeMax"`
			VolumeNet         float64 `json:"volumeNet"`
			VolumeRemaining   float64 `json:"volumeRemaining"`
			VolumeReserved    int     `json:"volumeReserved"`
			VolumeUsed        float64 `json:"volumeUsed"`
			VolumeUtilization float64 `json:"volumeUtilization"`
			WeightMax         float64 `json:"weightMax"`
			WeightNet         int     `json:"weightNet"`
			WeightRemaining   float64 `json:"weightRemaining"`
			WeightTare        float64 `json:"weightTare"`
			WeightUsed        float64 `json:"weightUsed"`
			WeightUtilization float64 `json:"weightUtilization"`
		} `json:"box"`
	} `json:"boxes" bson:"boxes"`
	Built  string `json:"built" bson:"built"`
	Host   string `json:"host" bson:"host"`
	Images []struct {
		BoxIndex int    `json:"boxIndex"`
		Format   string `json:"format"`
		Data     string `json:"data"`
	} `json:"images" bson:"images"`
	ItemSortReverseUsed    bool     `json:"itemSortReverseUsed" bson:"itemSortReverseUsed"`
	ItemSortUsed           string   `json:"itemSortUsed" bson:"itemSortUsed"`
	Leftovers              []any    `json:"leftovers" bson:"leftovers"`
	LenBoxes               int      `json:"lenBoxes" bson:"lenBoxes"`
	LenItems               int      `json:"lenItems" bson:"lenItems"`
	LenLeftovers           int      `json:"lenLeftovers" bson:"lenLeftovers"`
	LenUnits               int      `json:"lenUnits" bson:"lenUnits"`
	OrderID                string   `json:"orderId" bson:"orderID"`
	PackTime               float64  `json:"packTime" bson:"packTime"`
	PackUUID               string   `json:"packUuid" bson:"packUUID"`
	RenderTime             float64  `json:"renderTime" bson:"renderTime"`
	RequestFingerprint     string   `json:"requestFingerprint" bson:"requestFingerprint"`
	RequestID              string   `json:"requestId" bson:"requestID"`
	ResponseFingerprint    string   `json:"responseFingerprint" bson:"responseFingerprint"`
	Scripts                string   `json:"scripts" bson:"scripts"`
	SVGs                   []string `json:"svgs" bson:"svGs"`
	StartedAt              string   `json:"startedAt" bson:"startedAt"`
	Styles                 string   `json:"styles" bson:"styles"`
	Title                  string   `json:"title" bson:"title"`
	TotalCost              int      `json:"totalCost" bson:"totalCost"`
	TotalTime              float64  `json:"totalTime" bson:"totalTime"`
	TotalVolume            float64  `json:"totalVolume" bson:"totalVolume"`
	TotalVolumeNet         float64  `json:"totalVolumeNet" bson:"totalVolumeNet"`
	TotalVolumeUsed        float64  `json:"totalVolumeUsed" bson:"totalVolumeUsed"`
	TotalVolumeUtilization float64  `json:"totalVolumeUtilization" bson:"totalVolumeUtilization"`
	TotalWeight            float64  `json:"totalWeight" bson:"totalWeight"`
	UsedKeystem            string   `json:"usedKeyStem" bson:"usedKeystem"`
	Version                string   `json:"version" bson:"version"`
	Warnings               []string `json:"warnings" bson:"warnings"`
}

type ErrorResponse struct {
	Message string
}

type InformationResponse struct {
	Message string
}

const (
	pacUrl string = "https://api.paccurate.io/"
)

func main() {

	router := gin.Default()

	producer, err := initProducer(60 * time.Second)
	if err != nil {
		slog.Error(fmt.Errorf("couldn't open a connection to kafka: %s", err).Error())
		return
	}
	defer (*producer).Close()

	mongoClient, err := mongoutils.InitMongoSession()
	if err != nil {
		slog.Error(fmt.Errorf("couldn't open a mongodb connection: %s", err).Error())
		return
	}
	defer func() {
		if err = mongoClient.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()

	// Get aggregate data for one single keystem or from all as a AggregatedKeyStats object.
	router.GET("/api/keydata/:keystem", func(c *gin.Context) {
		keystem := c.Param("keystem")
		var err error
		if keystem == "all" {
			akStats, err := mongoutils.GetAggregatedKeyStats(mongoClient)
			if err == nil {
				c.JSON(200, akStats)
				return
			}
		} else {
			keyStats, err := mongoutils.GetKeyStats(mongoClient, []string{keystem})
			if err == nil {
				c.JSON(200, keyStats[keystem])
				return
			}
		}
		if err != nil {
			c.JSON(500, ErrorResponse{fmt.Errorf("error encountered while fetching stats: %s", err).Error()})
		}
	})

	// Delete aggregated data for one keystem
	router.DELETE("/api/keydata/:keystem", func(c *gin.Context) {
		keystem := c.Param("keystem")
		deleteRequest := statistics.DeleteRequest{keystem}
		sendMessage(producer, &deleteRequest, config.MessageTypeDelete)

		c.JSON(200, InformationResponse{fmt.Sprintf("delete request sent on keystem %s", keystem)})
	})

	// Forward a POST request to the Paccurate API
	// This returns a statistical summary of that individual pack request
	router.POST("/", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			err = fmt.Errorf("error occurred reading response body: %s", err)
			slog.Error(err.Error())
			c.JSON(500, ErrorResponse{err.Error()})
			return
		}

		// Validate Pack API requests
		var packRequest PackRequest
		err = json.Unmarshal(body, &packRequest)
		if err != nil {
			err = fmt.Errorf("malformed request")
			slog.Error(err.Error())
			c.JSON(400, ErrorResponse{err.Error()})
			return
		}

		validate := validator.New()
		err = validate.Struct(packRequest)
		if err != nil {
			err = fmt.Errorf("invalid request")
			slog.Error(err.Error())
			c.JSON(422, ErrorResponse{err.Error()})
			return
		}

		stats := statistics.NewStats()

		// TODO check if request is for a random pack. If not, check redis cache
		// If this request wasn't found in the cache, then we forward it to Paccurate
		if !stats.CacheHit {
			proxyReq, err := createProxyRequest(c.Request.Header, body)
			if err != nil {
				err = fmt.Errorf("error occurred creating proxy request: %s", err)
				slog.Error(err.Error())
				c.JSON(500, ErrorResponse{err.Error()})
				return
			}
			client := http.Client{}
			timeStart := time.Now()
			resp, err := client.Do(proxyReq)
			latency := time.Since(timeStart)
			stats.TimeStamp = timeStart.String()
			stats.Latency = float64(latency)
			if err != nil {
				stats.RequestError = true
				err = fmt.Errorf("error occurred forwarding request to Paccurate: %s", err)
				slog.Error(err.Error())
				c.JSON(resp.StatusCode, ErrorResponse{err.Error()})
				return
			}
			if resp != nil {
				stats.StatusCode = strconv.Itoa(resp.StatusCode)
				if resp.StatusCode >= 400 {
					stats.ErrorResponse = true
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					slog.Error(err.Error())
					err = fmt.Errorf("error occurred reading response body: %s", err)
					slog.Error(err.Error())
					c.JSON(500, ErrorResponse{err.Error()})
					return
				}
				err = analyzePackResponseBody(body, stats)
				if err != nil {
					err = fmt.Errorf("error occurred analyzing response body: %s", err)
					slog.Error(err.Error())
					c.JSON(500, ErrorResponse{err.Error()})
					return
				}
			}
		}
		err = sendMessage(producer, stats, config.MessageTypeStats)
		if err != nil {
			slog.Error(err.Error())
		}
		c.JSON(200, stats)
	})

	router.Run()
}

// createProxyRequest() creates a new request by recycling the initial request.
// The new request is sent to Paccurate.
func createProxyRequest(header http.Header, body []byte) (*http.Request, error) {
	proxyReq, err := http.NewRequest("POST", pacUrl, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	proxyReq.Header.Set("Authorization", header["Authorization"][0])
	proxyReq.Header.Set("accept", "application/json")
	proxyReq.Header.Set("Content-Type", "application/json")
	return proxyReq, nil
}

// analyzePackResponseBody parses pack response body and extract statistics
func analyzePackResponseBody(body []byte, stats *statistics.Stats) error {
	var packResponse PackResponse

	err := json.Unmarshal(body, &packResponse)
	if err != nil {
		return err
	}

	stats.UsedKeystem = packResponse.UsedKeystem
	stats.TotalItems = packResponse.LenItems
	stats.TotalVolume = packResponse.TotalVolume
	stats.VolumeUtilization = packResponse.TotalVolumeUtilization

	for _, box := range packResponse.Boxes {
		refId := strconv.Itoa(box.Box.BoxType.RefID)
		_, keyExists := stats.BoxTypes[refId]
		if !keyExists {
			stats.BoxTypes[refId] = 0
		}
		stats.BoxTypes[refId]++
	}
	return nil
}
