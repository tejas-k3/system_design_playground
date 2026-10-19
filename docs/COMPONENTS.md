# Components and how they fail

What each component in the simulator does, what makes it fail, and why you can trust (or question) those rules. Every statement here was checked against the engine source (`backend/internal/engine/`) and the tests named in each section. If the code and this file disagree, the code is right and this file is a bug.

## The idea in one paragraph

A request is a chain of calls. Every component it touches holds a **resource** for a while (a thread, a database slot, a token), and when the resource is busy, the request **waits in a queue**. When the queue is full, or the wait is longer than the caller will tolerate, the request **fails** with a named reason. Almost nothing "breaks" at a fixed number: a server does not fail at 400 requests a second, it fails because at that rate its threads stay busy, its queue fills, answers arrive after the user has given up, and those become the failures you see. Only a few limits are explicit settings (queue sizes, connection caps, the rate limiter's rate, thread counts). Everything else is emergent from them.

The simulation is a deterministic discrete-event engine: the same design and the same seed always give the same result.

## Components

Defaults are the values a component starts with; every number is a setting you can change in the inspector.

| Component | Concurrency (what it can work on at once) | Queue before it refuses | What happens when it fails |
|---|---|---|---|
| **Users** | not a server: arrival rate (default 50 req/s; open, bursty or a closed-loop crowd) | none | The user gives up after `timeoutMs` (2,000 ms): the request fails as **Timeouts**. Retries (default 0) add load exactly when the system is struggling; the abandoned attempt keeps consuming resources downstream. |
| **Load Balancer** | effectively unlimited: it is never the bottleneck in this model | none | It fails a request with **No healthy backend** when every backend is down or ejected. A crashed server keeps receiving traffic until the health check notices (default 2 s), and those requests fail as **Node down**. Round robin, least connections or random; optional outlier ejection. |
| **App Server** | `threads` (8) x servers running; each request holds a thread for its whole life, including time spent waiting on a dependency | `queueCap` (50), first-in-first-out or newest-first; optional maximum wait | Threads busy and queue full: **Queue full (503)**. Waited longer than the maximum queue wait: **Waited too long in queue**. A dependency call with no call timeout (default: wait forever) keeps its thread, so a silent dependency fills all threads. With a call timeout: **Call timeouts (504)**. A tripped circuit breaker: **Circuit open**. A crash fails every request in flight on it (**Node down**). Optional stop-the-world GC pauses (`gcPauseMs`, mean every `gcEverySec`, random gaps): for the pause nothing starts or finishes, so latency grows in a tail while the median barely moves. Optional autoscaling adds whole servers after a boot delay. |
| **Cache** | 10,000 at once | 2,000 | It rarely fails. A **down** cache is bypassed: every lookup becomes a miss and goes to the data store, so the store takes the whole load. A small or cold cache has a low hit rate; entries that all expire together cause a stampede (see the cache-stampede challenge). |
| **Database** | `connections` (16 concurrent queries) | `queueCap` (100) | Slots busy and queue full: **Queue full (503)**. More open connections than `maxConnections` (500) across all servers: **DB connections refused**. Crashed: **Node down**, or **No primary: failing over** while a replica is being promoted. Isolation (for the users' Transfers workload): read committed lets two transfers overwrite each other (counted as overwritten updates), repeatable read aborts the second committer (**Transaction aborted**), serializable locks both accounts so transfers wait for each other and deadlocks abort a victim; throughput falls as isolation rises under contention. Data model preset (default relational): document makes single-key reads and writes about 20% cheaper; wide-column writes cost 40% but a lookup by another field scans every partition (3x the scan); time-series writes cost 30% and random point reads 50% more. These are coarse multipliers, not a model of the engines. Storage engine (default B-tree): an LSM engine makes writes about half as costly and reads about 1.5x as costly, and pauses writes for a stall (default 400 ms every 30 s) while it compacts, so its write tail grows while the median stays low (a **compaction** event marks each one). Data size (default off): when `dataGB` exceeds the buffer pool, the missing share of pages is read from disk at 10x the cost. Secondary indexes (default 0): each makes every write 30% slower, and a lookup by another field (the users' `secondaryPct`) costs `readMs x scanMult` (default x20) when there is no index. Failover has a detection time and a promotion time; writes newer than the replica's lag are lost (counted), and an unfenced old primary that returns causes split brain. |
| **Read Replica** | 16 | 100 | Same limits as the database. Reads can be **stale** (counted, not failed). Synchronous replication makes every write wait for it (**Sync replica unreachable** if it is down). |
| **Message Queue** | n/a: it only buffers | `capacity` (5,000 jobs) | A full queue refuses new messages (**Queue full (503)**). A message larger than its limit (`maxMessageKB`, 256 KB) is refused as **Message too large**, but only when the users set a request size (`reqKB`; 0 means "not modelled"). It only accepts writes. The backlog grows whenever workers are slower than arrivals. |
| **Worker** | `concurrency` (4) | the queue it reads from | It does not fail requests; it falls behind. Watch the backlog and job latency. |
| **CDN** | effectively unlimited | none | It does not fail; misses go to the origin, so a cold start or a short TTL sends a burst there (purging is planned, #11). CDNs can be chained (an edge CDN in front of a shield CDN in front of the origin); a tier with *collapse concurrent misses* sends one request to the origin for a key however many edges miss on it at once. A purge chaos event empties one. |
| **Rate Limiter** | token bucket: `limitRps` (200) and `burst` (50) | none | Above the rate it rejects on purpose: **Rate limited (429)**. |
| **Shard Router** | n/a: it routes by key | none | It does not fail by itself. A hot key saturates one database while the others idle; a dead shard fails only its keys. |
| **External API** | unlimited (only latency: 80 ms, and an error rate: 1%) | none | Errors are **Vendor errors**. A slow vendor ties up the threads of the servers that wait on it. |
| **Object Store** | `maxConcurrent` (1,000) uploads | none | An app server connected to one uploads the body of each write there first (time to first byte 30 ms plus the body at 50 MB/s per upload, over the link's bandwidth); from then on only a pointer travels, so a queue's message limit no longer applies (the claim-check pattern). Reads never touch it. It stops answering if it crashes (**Node down**, which fails the upload). |
| **Search Shard** | `connections` (16 queries at once) | `queueCap` (50) | One slice of a search index. With the app server's fan-out set to *all*, a read goes to every shard and waits for the last one, so latency is the slowest of N and one dead shard fails every query. *First K* answers once K shards have replied (the others still do the work); *hedging* asks a slow shard again after `hedgeAfterMs` (the same shard, an independent service-time draw, extra load); *partial results* reply with what there is when too few answered, counted as partial. Fan-out is off by default and shards are not used for reads then. |
| **Redis** | 1 (single-threaded) | 2,000 | Queue full: **Queue full (503)**. A lock that is held: **Lock busy**. With persistence off, a crash loses what it held. |

### Failure reasons you will see

Timeouts, Queue full (503), Node down, No route, No healthy backend, Rate limited (429), Call timeouts (504), Circuit open, Vendor errors, Lock busy, Waited too long in queue, DB connections refused, No primary: failing over, Sync replica unreachable, Message too large, Connection lost (six packets in a row lost on a lossy link), Transaction aborted. The chart shows them in "Why requests failed", and the Request traces tab explains one request at a time.

## Chaos you can inject

Crash (with automatic restart), slow-down (steady or pulsing), network partition (a node that never answers, not one that refuses), and for databases a failover with detection and promotion times. A crash fails the requests already in flight on that node.

## Why these rules are believable

- **Queueing theory.** A single-server queue is tested against the Pollaczek-Khinchine formula and a multi-server queue against the Allen-Cunneen approximation (`queueing_test.go`, `variability_test.go`). The mean wait the engine produces matches those results within a stated tolerance across several loads.
- **Determinism.** Same diagram and seed, same result, so a failure you see can be reproduced, rewound (the rewind slider replays exactly) and compared.
- **Each behaviour is a checked scenario.** A retry storm that outlives its trigger, a cache stampede, a stuck-thread outage behind a silent database, split brain after failover, an autoscaler that boots too slowly: each is a challenge whose starting design must fail and whose fix must pass, with identical stars on seeds 1-8 (`scenarios_test.go`). Several first drafts were not winnable and were retuned.
- **Capacity numbers are measured, not guessed.** For example, three 16-thread app servers behind a cache and a database carried about 1,157 requests a second (the "roughly 400 per server" used by the interview calculator).
- **The design lint reproduces the same patterns** statically (`lint.go`), and its rules are cross-checked against the challenge designs.

## What the defaults are, and are not

The defaults (8 threads, 16 database slots, a 50-request queue, and so on) are round, plausible numbers chosen so that interesting failures appear at loads that run comfortably in a browser. They are not benchmarks of any real server or database, and you should not read "a server handles 400 requests a second" as a fact about the world. The *shape* (utilisation rises, queues build, timeouts follow, retries amplify) is the point.

## What is not modelled yet

downloads from the object store, and storage or egress cost (replicas per search shard; a link can have a bandwidth limit, a queue a message limit and a cache or CDN a byte capacity, once the users set request or response sizes); clock skew and fsync loss of unflushed writes (disk IOPS as a queue for writes exists; GC pauses, packet loss, jitter, CPU burst credits and out-of-memory kills exist); write skew (isolation levels, data volume, storage engines and coarse data-model presets exist; secondary indexes and isolation levels exist); regions, zones and deployments; consensus and distributed data; stream windows and approximate data structures; LLM serving. These are tracked as GitHub issues #11, #13, #16, #17, #18, #19, #20 and #21.

## Check it yourself

```
make test                      # engine, scenarios (every challenge's start fails, solution passes) and API
cd backend && go test ./internal/engine -run 'Pollaczek|AllenCunneen' -v
make browser-check             # the browser regressions, in headless Chrome
```
