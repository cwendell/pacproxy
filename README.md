
## Deployment
To deploy this project and all of its services, navigate to the root of this project and use `docker compose -f docker-compose.yml up`.

## API Usage

### Send a pack request to the proxy

```bash
curl -X 'POST' \
  'http://localhost:8080' \
  -H 'accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{
  "requestId": "string",
  "orderId": "string",
  "layFlat": false,
  ...
```
This returns statistics for that pack request:
```json
{
    "usedKeystem":"aqRAiz-8RA",         // Keystem used, uniquely identifying the facility
    "totalItems":23,                    // Total items for this pack
    "totalVolume":17280,                // Total volume for this pack
    "volumeUtilization":0.36996528,     // Volume utilization for this pack
    "boxTypes":{"0":12},                // Map of box refIds to the amount of that box type used
    "timeStamp":"2025-03-25 05:47:23.483992326 +0000 UTC m=+15.240067993",  // Timestamp of request, taken at the time when the proxy forwarded the request
    "cacheHit":false,                   // True if the request was found in the cache - unused
    "requestError":false,               // True if there was an error forwarding the request to Paccurate
    "errorResponse":false,              // True if the proxy received an error response from Paccurate
    "statusCode":"200",                 // HTTP status code of the response from Paccurate
    "latency":621623810                 // Round trip latency of the request
}
```

### Send a request to fetch data aggregated by keystem
```bash
curl -X GET http://localhost:8080/api/keydata/{keystem}
```
Example response:
```json
{
    "usedKeystem":"aqRAiz-8RA",         // Keystem used, uniquely identifying the facility
    "totalItems":69,                    // Total items across all packs
    "totalVolume":51840,                // Total volume across all packs
    "avgItemsPerPack":23,               // Average items per pack
    "avgVolumeUtilization":0.36996528,  // Average volume utilization per pack
    "boxTypes":{"0":36},                // Map of box refIds to the amount of that box type used
    "totalRequests":3,                  // Total number of requests
    "statusCodes":{"200":3},            // Map of status codes and their frequency
    "requestErrorCount":0,              // Total number of errors encountered reaching Paccurate
    "errorCount":0,                     // Total error responses received from Paccurate
    "cacheHits":0,                      // Total number of cache hits (unused)
    "maxLatency":{
        "latency":930001561,            // Value of the highest request latency 
        "timeStamp":"2025-03-25 05:47:17.191713585 +0000 UTC m=+8.947789246" // Timestamp of the highest latency
    },
    "avgLatency":689424478.3333334      // Average latency
}
```

### Send a request to fetch a summary of all data across all keystems
```bash
curl -X GET http://localhost:8080/api/keydata/all
```
Example response:
```json
{
    "totalItems": 69,                   // Total items across all packs
    "totalVolume": 51840,               // Total volume across all packs
    "avgItemsPerPack": 23,              // Average items per pack
    "avgVolumeUtilization": 0.36996528, // Average volume utilization per pack
    "boxTypes": {"0": 36},              // Map of box refIds to the amount of that box type used
    "totalRequests": 3,                 // Total number of requests
    "statusCodes": {"200": 3},          // Map of status codes and their frequency
    "requestErrorCount": 0,             // Total number of errors encountered reaching Paccurate
    "errorCount": 0,                    // Total error responses received from Paccurate
    "cacheHits": 0,                     // Total number of cache hits (unused)
    "maxLatency": {
        "latency": 930001561,           // Value of the highest request latency 
        "usedKeystem": "aqRAiz-8RA",    // Keystem of the facility who experienced the highest latency
        "timeStamp": "2025-03-25 05:47:17.191713585 +0000 UTC m=+8.947789246"   // Timestamp of the highest latency
    },
    "avgLatency": 689424478.3333334,    // Average latency
    "highestAvgLatency": { 
        "latency": 689424478.3333334,   // Value of the highest average request latency
        "usedKeystem": "aqRAiz-8RA"     // Keystem of the facility that experienced the highest average request latency
    }
}
```

### Send a request to clear all historical data for a keystem
```bash
curl -X DELETE http://localhost:8080/api/keydata/{keystem}
```
This returns a confirmation message. Note that the data may not be cleared immediately.
```json
{"Message":"delete request sent on keystem aqRAiz-8RA"}
```

If an error is encountered, a corresponding status code will be returned as well as a message describing the error:
```json
{"Message":"malformed request"}
```

# My approach

# Scale
For this problem, we need to think about scale. The assignment sheet says we're 
dealing with a large e-commerce company. Right away, my thoughts went to Amazon as 
a real-life analog. Amazon processes almost six billion orders per year. Almost
two hundred every second from their approximately 300 warehouses worldwide.
Our system will have to process statistics for each of those 200 requests per 
second. Of course, that's just an average, and we'll need to be able to handle spikes
as well. I asked, and I was told, that the statistical summaries of these requests, the
result of all of our processing, will only be viewed once in a while by a handful of
people. So, if we're writing and reading from a database, writes will account for the 
vast majority of our traffic.

# Fault tolerance and correctness:
Our system needs to continue working if one of its parts fails, and no data must be 
lost. Our aggregated statistics probably don't need to be up to date the instant the end
user clicks the 'load' button - they can be a second or three behind - but we do need to keep track of all of them, and they all need to be factored into the final scores eventually.

## Bad solutions and decent solutions

**Just count the packs** - We process packs and sum the statistics into a central data store
as they come in. If we required up-to-date statistics at all times, this might be one of the 
better solutions. But as it is, this isn't necessary. Two hundred requests per second will
inevitably lead to race conditions as the data store is rapidly updated. Locking can solve that, but it'll also slow things down. We can scale by partitioning our writes across multiple nodes assigned to different keystems, but then any aggregation beyond that would require input from
multiple nodes, which isn't ideal.

**The long ledger** - We take advantage of our low read requirements by writing all of our requests to a timeseries database. Fast writes, slow reads. Very slow reads. I've never had to
wait for a query to aggregate a hundred million records, and I hope I'll never need to. Our read
requirements might be low, but we can probably do better than that. We can improve on this idea by aggregating in batches - a few dozen or a few hundred packs aggregated and then summed with the existing contents of a central data store.  

**My solution: stream processing** - Instead of writing directly to a database, we write statistics for each individual pack to a distributed, persistent write-ahead log. This log is processed in batches, and those batches are aggregated periodically (every few seconds) to a central data store. Before this project, I had no experience with distributed stream processing, but I figured it wouldn't hurt to learn. I now regret that optimism.

I started by doing some research, and landed on a tool called Kafka, licensed by Apache. Kafka is a distributed, fault tolerant stream processing platform. It was almost exactly what I needed, and learning about how to use it made this project much easier, and much harder. My application servers dump pack stats into Apache Kafka, and on the other side of the queue, consumers aggregate the data and write it in batches to a central database. In the database, the consumers (a.k.a aggregators) leave an offset marker of where they last read from the event queue. If something breaks and the system needs to be reset, the aggregators are programmed to read that offset marker and send a request to Kafka to replay the event logs from that point forward.

Here's how my project can scale:
- We use the keystem from each pack request to spread the load relatively evenly across all resources, ensuring the data from any given facility always goes through the same set of nodes.
- First, our application servers, after processing a pack request, send the resulting statistics to one of many Kafka partitions based on the hash of the keystem.
- Next, each aggregator will consume from one or more Kafka partitions, with no two reading from the same Kafka partition. Kafka handles this partition assignment, and the aggregators will reset themelves when they're reassigned. These partitions should be replicated to prevent data loss.
- Finally, each aggregator writes their newest batches of data to a centralized database, each record aggregated around a single keystem. This database should use single-leader replication to prevent data loss in the event of competing overwrites. Each aggregator can handle data from many facilities, and each aggregator only writes to the database every few seconds (or more if there are lots of aggregators), so this can work at scale. And because data for each keystem will always be written to the same Kafka partition, we can store the offset (monotonically incrementing sequence ID of each Kafka messages) of each partition in our central database, allowing us to pick back up after a failure without missing or double-counting data. This also means that aggregators will be writing over different data, which means we won't see competing writes.

So, our centralized database takes aggregated statistics in batches, with breaks in between writes. Because most of the heavy lifting is done by Kafka, we don't need to worry too much about choosing a database based on performance. I chose based on convenience, and I chose MongoDB. Its JSON-like stucture makes it easy to store some complex aggregated statistics if we ever choose to, and it made it easy to process data from the message queues without too much rearranging on my end.

## Final thoughts
I had a good time with this project, and I feel good about the work I did. It isn't perfect, and there are failure points. The most glaringly obvious example is the lack of a process to resend unacknowledged messages to Kafka, or even the implementation of idempotency for this process, which Kafka does support. Unfortunately, I didn't have the time to implement it, and this was a common theme. There was a lot I wanted to do, and I was never going to be able to do all of it. And my work on these half-finished features took away from my work on basic optimizations. I'm glad I gave this project my all, but in the future, I'll probably scale back my grand ideas and focus on implementing a simpler design, and implementing it well.

### Things to fix

**Optimize aggregation** - Currently, aggregation is done in completely by our servers and aggregators, and not by our database. Currently, aggregators will overwrite keystem-specific records instead of passing aggregated data to be handled by the database. Currently, statistical summaries across all keystems are generated by querying data from all keystems and aggregating them in the application server. This is far from ideal.

Ideally, if we're going to have a tool like Prometheus constantly requesting statistics, we should have a constantly-updated record of the all-keystems statistical summary. When the aggregators pass new batches to MongoDB, the database should aggregate and update the documents for the relevant keystems, and then it should update the all-keystems summary. This can and should be done using an aggregation pipeline or with complex queries. In my case, I had a tight deadline, and I'm faster at writing code in Go than I am at writing code for MongoDB.

**Implement request caching** - Throughout the project, you'll notice references to request caching, and even aggregation logic to handle cache hits, despite the conspicuous lack of said caching in the project's current state. This could be handled with something like Redis, an in-memory key-value store which can handle keys the size of our Paccurate requests.

**Implement indexes in MongoDB** - You might notice that nowhere do we create any indexes to speed up lookups on keystem or partition ID. If this were a real product I'd definitely want to implement those, and this would be done with an initialization script, run when the container is being created for the first time.
