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
	NumReasons
)

var reasonNames = [NumReasons]string{
	"timeout", "queueFull", "nodeDown", "noRoute", "noBackend", "rateLimited",
	"callTimeout", "circuitOpen", "dependencyError", "lockBusy", "queueTimeout", "connRefused",
	"noPrimary", "syncReplica", "messageTooLarge", "connLost", "txAborted",
}

var reasonLabels = [NumReasons]string{
	"Timeouts", "Queue full (503)", "Node down", "No route", "No healthy backend", "Rate limited (429)",
	"Call timeouts (504)", "Circuit open", "Vendor errors", "Lock busy", "Waited too long in queue", "DB connections refused",
	"No primary: failing over", "Sync replica unreachable", "Message too large", "Connection lost", "Transaction aborted",
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
	Type        string  `json:"type"` // crash | slow | partition
	NodeID      string  `json:"nodeId"`
	AtSec       float64 `json:"atSec"`
	DownSec     float64 `json:"downSec,omitempty"`
	Factor      float64 `json:"factor,omitempty"`
	DurationSec float64 `json:"durationSec,omitempty"` // slow and partition
	PulseSec    float64 `json:"pulseSec,omitempty"`    // slow only: alternate slow and normal every PulseSec until DurationSec is over
}

// TrafficPoint is (seconds, rps) on a piecewise-linear traffic curve.
type TrafficPoint [2]float64
