// Package engine is the deterministic discrete-event simulator behind System Design Playground. It does no I/O.
//
// A [Diagram] (nodes, edges, notes) is checked and clamped by [Sanitize] and then run by a [Sim]: clients
// generate requests (traffic curves, retries, closed or open arrival), and each request walks the diagram through
// stations, pools and balancers, with the behaviour of every component kind (app server, cache, database, queue,
// stream, coordinator and so on) in its own file. Chaos events crash, slow or partition components during a run.
//
// The registry ([Specs]) holds one [NodeSpec] per component kind: its settings with ranges, defaults and help
// text, and its cost. The same data validates a diagram and renders the settings panel in the browser.
//
// A run is summarised as [RunStats]. [Goals] and [Evaluate] turn those statistics into checks and a one-to-three
// star [Score], [RunToEnd] runs a design to the end of its traffic and scores it, and [Lint] reports design
// smells without running anything.
//
// Determinism is a hard requirement: the same diagram and seed give an identical result. All randomness comes from
// the Sim's seeded generators, a feature that is off consumes no draw, and map iteration order never decides a
// result.
package engine
