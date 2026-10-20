package engine

// Node kinds.
const (
	KClient       = "client"
	KLoadBalancer = "loadBalancer"
	KAppServer    = "appServer"
	KCache        = "cache"
	KDatabase     = "database"
	KReplica      = "readReplica"
	KQueue        = "queue"
	KWorker       = "worker"
	KCDN          = "cdn"
	KRateLimiter  = "rateLimiter"
	KShardRouter  = "shardRouter"
	KExternalAPI  = "externalApi"
	KRedis        = "redis"
	KObjectStore  = "objectStore"
	KIndex        = "index"
	KTopic        = "topic"
	KLog          = "log"
	KKVStore      = "kvStore"
)

// Params holds user-editable settings: numbers, strings (select) and booleans.
type Params map[string]any

type Node struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Label  string  `json:"label"`
	Params Params  `json:"params"`
	Locked bool    `json:"locked,omitempty"`
}

type Edge struct {
	ID        string  `json:"id"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	LatencyMs float64 `json:"latencyMs"`
	// BandwidthMbps limits the link (0 = unlimited). Bytes only exist when the users set reqKB/respKB; with no sizes
	// there is nothing to carry and the limit changes nothing.
	BandwidthMbps float64 `json:"bandwidthMbps,omitempty"`
	// LossPct is the chance a packet is lost on each trip (a lost one is retransmitted after 200 ms; six losses in a
	// row abort the connection). JitterMs adds a random 0..JitterMs to each one-way trip. Both 0 = a perfect link.
	LossPct  float64 `json:"lossPct,omitempty"`
	JitterMs float64 `json:"jitterMs,omitempty"`
	// Protocol and Async are labels for the reader (http, grpc, tcp, ws, sql, custom; a dashed arrow for fire-and-forget
	// calls). The simulation ignores both.
	Protocol string `json:"protocol,omitempty"`
	Async    bool   `json:"async,omitempty"`
}

type Note struct {
	ID       string  `json:"id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Text     string  `json:"text"`
	FontSize float64 `json:"fontSize,omitempty"`
}

type Diagram struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
	Notes []Note `json:"notes"`
}

// Reason is why a request failed at some node.
type Reason int

const (
	RTimeout Reason = iota
	RQueueFull
	RNodeDown
	RNoRoute
	RNoBackend
	RRateLimited
	RCallTimeout
	RCircuitOpen
	RDependency
	RLockBusy
	RQueueTimeout
	RConnRefused
	RNoPrimary
	RSyncReplica
	RMessageTooLarge
	RConnLost
	RTxAborted
	RPoisonJob
	RBadRelease
	RBulkheadFull
	RDeadline
	RShed
	RQuorum
	NumReasons
)

var reasonNames = [NumReasons]string{
	"timeout", "queueFull", "nodeDown", "noRoute", "noBackend", "rateLimited",
	"callTimeout", "circuitOpen", "dependencyError", "lockBusy", "queueTimeout", "connRefused",
	"noPrimary", "syncReplica", "messageTooLarge", "connLost", "txAborted", "poisonJob", "badRelease", "bulkheadFull", "deadlinePassed", "shed", "quorum",
}

var reasonLabels = [NumReasons]string{
	"Timeouts", "Queue full (503)", "Node down", "No route", "No healthy backend", "Rate limited (429)",
	"Call timeouts (504)", "Circuit open", "Vendor errors", "Lock busy", "Waited too long in queue", "DB connections refused",
	"No primary: failing over", "Sync replica unreachable", "Message too large", "Connection lost", "Transaction aborted", "Poison job", "Bad release (5xx)", "Bulkhead full", "Deadline passed (skipped)", "Shed (load too high)", "Not enough copies for a quorum",
}

// ReasonInfo names a failure reason for clients: a stable id and a human label.
type ReasonInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ReasonInfos lists every failure reason so clients need not hard-code their names.
func ReasonInfos() []ReasonInfo {
	out := make([]ReasonInfo, NumReasons)
	for r := Reason(0); r < NumReasons; r++ {
		out[r] = ReasonInfo{ID: reasonNames[r], Label: reasonLabels[r]}
	}
	return out
}

func (r Reason) String() string {
	if r < 0 || r >= NumReasons {
		return "unknown"
	}
	return reasonNames[r]
}

// ChaosEvent is a scheduled failure injected into a run.
type ChaosEvent struct {
	Type        string  `json:"type"` // crash | slow | partition | linkDown | purge | zoneDown | badDeploy | rollingRestart | kvNodeDown (Factor = how many nodes of the cluster)
	NodeID      string  `json:"nodeId"`
	AtSec       float64 `json:"atSec"`
	DownSec     float64 `json:"downSec,omitempty"`
	Factor      float64 `json:"factor,omitempty"`
	DurationSec float64 `json:"durationSec,omitempty"` // slow and partition
	Version     string  `json:"version,omitempty"`     // badDeploy: only components running this version (v1 | v2) get the bad release
	Kind        string  `json:"kind,omitempty"`        // badDeploy and rollingRestart: which kind of component
	FractionPct float64 `json:"fractionPct,omitempty"` // badDeploy: share of that kind that gets the bad release (at least one)
	IntervalSec float64 `json:"intervalSec,omitempty"` // rollingRestart: gap between one restart and the next
	Zone        string  `json:"zone,omitempty"`        // zoneDown only: every node placed in this zone crashes for DurationSec
	ToID        string  `json:"toId,omitempty"`        // linkDown only: the connection from NodeID to this node
	Mode        string  `json:"mode,omitempty"`        // linkDown only: both | replies (requests arrive, answers are lost) | requests
	PulseSec    float64 `json:"pulseSec,omitempty"`    // slow only: alternate slow and normal every PulseSec until DurationSec is over
}

// TrafficPoint is (seconds, rps) on a piecewise-linear traffic curve.
type TrafficPoint [2]float64
