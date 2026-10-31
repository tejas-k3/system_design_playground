package scenarios

func init() {
	registerQuestion(&Question{
		ID: "network-assumptions", Title: "Which assumption broke?", Mode: ModeMatch, Difficulty: "Medium",
		Summary: "Eight familiar assumptions, eight ways a distributed design can fail.",
		Prompt:  "Each case describes a failure from a system design. Choose the assumption it exposes.",
		Items: []MatchItem{
			{Need: "In Cut off, a partitioned database never answers, and app threads wait until they are exhausted.", Options: []string{"The network is secure", "The network is reliable", "The network is homogeneous", "There is one administrator"}, answer: 1, why: "A network path can stop delivering useful responses without a clean refusal. Timeouts and failure handling are needed even when a component has not crashed."},
			{Need: "In Bot flood, a scraper sends far more traffic than real customers and consumes shared capacity.", Options: []string{"The network is reliable", "The network is homogeneous", "The network is secure", "Transport cost is zero"}, answer: 2, why: "The system must account for hostile or abusive callers. This challenge models abusive traffic and rate limiting, not encryption or authentication."},
			{Need: "In One slow server, one of four servers runs eight times slower than its peers, yet the balancer still hands it a quarter of the traffic.", Options: []string{"The network is homogeneous", "The topology does not change", "Bandwidth is infinite", "There is one administrator"}, answer: 0, why: "The machines behind one name are not all alike: one can be slower, older or busier than the rest. A balancer that treats every server as equal keeps sending the slow one a full share."},
			{Need: "In Adding a shard under load, a fifth database joins while traffic flows, and most keys change owner.", Options: []string{"The network is secure", "Transport cost is zero", "The network is reliable", "The topology does not change"}, answer: 3, why: "The set of machines changes while the system runs. A placement rule that assumes a fixed set, such as hash mod n, has to move most keys when the set changes."},
			{Need: "In Multi-region reads, users far from the database wait for cross-ocean round trips.", Options: []string{"Latency is zero", "Bandwidth is infinite", "The network is homogeneous", "There is one administrator"}, answer: 0, why: "Propagation delay grows with distance, before the application or database does any work. More servers in the distant region do not remove that delay."},
			{Need: "In Video uploads, the offered video traffic exceeds the configured app-to-store link capacity.", Options: []string{"Transport cost is zero", "The network is secure", "Bandwidth is infinite", "The network is reliable"}, answer: 2, why: "A link has finite throughput, so traffic queues or fails when its sustained rate exceeds capacity. The challenge raises the link limit to carry the offered load."},
			{Need: "In Image feed bill, serving popular images creates a charge for data sent out of object storage.", Options: []string{"There is one administrator", "The network is homogeneous", "Transport cost is zero", "The topology does not change"}, answer: 2, why: "Moving data across a provider boundary can cost money. The challenge measures object-store egress and uses a CDN to reduce repeated origin transfers."},
			{Need: "In Waiting on a vendor, the email provider is slow and its service is outside your team's control.", Options: []string{"The network is reliable", "Latency is zero", "Bandwidth is infinite", "There is one administrator"}, answer: 3, why: "A third-party provider has its own operator, limits and failures. Put non-essential work behind a queue so its behaviour does not hold every user request open."},
		},
	})
}
