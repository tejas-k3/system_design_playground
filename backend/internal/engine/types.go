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
	NumReasons
)

var reasonNames = [NumReasons]string{
	"timeout", "queueFull", "nodeDown", "noRoute", "noBackend", "rateLimited",
	"callTimeout", "circuitOpen", "dependencyError", "lockBusy", "queueTimeout", "connRefused",
}

var reasonLabels = [NumReasons]string{
	"Timeouts", "Queue full (503)", "Node down", "No route", "No healthy backend", "Rate limited (429)",
	"Call timeouts (504)", "Circuit open", "Vendor errors", "Lock busy", "Waited too long in queue", "DB connections refused",
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
	Type        string  `json:"type"` // crash | slow
	NodeID      string  `json:"nodeId"`
	AtSec       float64 `json:"atSec"`
	DownSec     float64 `json:"downSec,omitempty"`
	Factor      float64 `json:"factor,omitempty"`
	DurationSec float64 `json:"durationSec,omitempty"`
}

// TrafficPoint is (seconds, rps) on a piecewise-linear traffic curve.
type TrafficPoint [2]float64
