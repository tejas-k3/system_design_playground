package engine

import "fmt"

// addShard gives a shard router one more child, a copy of its first, as a scheduled event. The keys that now belong
// to it move, which is measured (keysMoved) and, with rebalanceSec set, slows the requests for them.
func (s *Sim) addShard(r *node) {
	ch := shardChildren(r)
	if len(ch) == 0 {
		return
	}
	src, edge := ch[0].to, ch[0]
	s.shardAdds++
	id := fmt.Sprintf("%s-new%d", src.id, s.shardAdds)
	p := Params{}
	for k, v := range src.p {
		p[k] = v
	}
	n := s.addNode(Node{ID: id, Kind: src.kind, Label: src.label + " (new)", X: src.x, Y: src.y + 90, Params: p})
	s.addEdge(Edge{ID: r.id + ">" + id, From: r.id, To: id, LatencyMs: edge.latency, BandwidthMbps: edge.bw})
	s.afterTopologyChange()
	s.noteReshard()
	s.logEvent("addShard", n, 0)
}
