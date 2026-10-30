package scenarios

import (
	"errors"
	"fmt"

	"sysdesign/internal/engine"
)

// askVariant is one answer to a clarifying question: a change to the users' behaviour, or to a goal, that the
// reference design is then run against.
type askVariant struct {
	users P
	goals func(*engine.Goals)
}

// askFlip ties a decisive question to a setting. With the same design, answer a passes and answer b does not
// (a test runs both), so the question changes what has to be built. says is shown after grading.
type askFlip struct {
	a, b askVariant
	says string
}

// AskOption is one clarifying question a candidate could ask. Which ones matter, and why, stay on the server.
type AskOption struct {
	Text     string `json:"text"`
	decisive bool
	why      string
	flip     *askFlip
}

type AskItemResult struct {
	Chosen   bool   `json:"chosen"`
	Decisive bool   `json:"decisive"`
	Explain  string `json:"explain"`
	Flip     string `json:"flip,omitempty"`
}

type AskResult struct {
	Items  []AskItemResult `json:"items"`
	Found  int             `json:"found"`  // decisive questions asked
	Wasted int             `json:"wasted"` // questions asked that change nothing
	Missed int             `json:"missed"` // decisive questions not asked
	Score  float64         `json:"score"`
	Stars  int             `json:"stars"`
}

// GradeAsk grades the questions a learner chose to ask. Each wasted pick costs half a found one.
func (q *Question) GradeAsk(chosen []int) (AskResult, error) {
	if q.Mode != ModeAsk {
		return AskResult{}, errors.New("not a requirements question")
	}
	if len(chosen) == 0 || len(chosen) > q.Picks {
		return AskResult{}, fmt.Errorf("choose between 1 and %d questions", q.Picks)
	}
	picked := make([]bool, len(q.Asks))
	for _, i := range chosen {
		if i < 0 || i >= len(q.Asks) || picked[i] {
			return AskResult{}, errors.New("choose each question at most once, from the list")
		}
		picked[i] = true
	}
	out := AskResult{Items: make([]AskItemResult, len(q.Asks))}
	decisive := 0
	for i, a := range q.Asks {
		it := AskItemResult{Chosen: picked[i], Decisive: a.decisive, Explain: a.why}
		if a.decisive {
			decisive++
			it.Flip = a.flip.says
			if picked[i] {
				out.Found++
			} else {
				out.Missed++
			}
		} else if picked[i] {
			out.Wasted++
		}
		out.Items[i] = it
	}
	if v := float64(out.Found) - float64(out.Wasted)/2; v > 0 && decisive > 0 {
		out.Score = v / float64(decisive)
	}
	switch {
	case out.Score >= 0.75:
		out.Stars = 3
	case out.Score >= 0.5:
		out.Stars = 2
	case out.Score >= 0.25:
		out.Stars = 1
	}
	return out, nil
}

// The five things a clarifying question can settle, each checked by running the reference design both ways.
func flipReads() *askFlip {
	return &askFlip{
		a:    askVariant{users: P{"readPct": 90.0}},
		b:    askVariant{users: P{"readPct": 40.0}},
		says: "With nine requests in ten reading, the cache answers most of them and the database copes. With four in ten, the writes alone are more than the database can commit and requests start to fail. The read share decides whether you need a cache or a stronger write path.",
	}
}

func flipPeak() *askFlip {
	return &askFlip{
		a:    askVariant{users: P{"rps": 600.0}},
		b:    askVariant{users: P{"rps": 1200.0}},
		says: "The same design carries 600 requests a second and fails at 1,200. You size for the busiest moment, not the average day, so the size of the peak decides how many servers and how much database you need.",
	}
}

func flipSkew() *askFlip {
	return &askFlip{
		a:    askVariant{users: P{"rps": 800.0, "keySpace": 1000000.0, "zipfS": 1.4}},
		b:    askVariant{users: P{"rps": 800.0, "keySpace": 1000000.0, "keyDist": "uniform"}},
		says: "When a few items take most of the requests, a small cache holds them and the database sees little. When requests are spread evenly over a million items the cache almost never has the one asked for, and the database is swamped. Skew decides whether a cache is worth having.",
	}
}

func flipLatency() *askFlip {
	return &askFlip{
		a:    askVariant{goals: func(g *engine.Goals) { g.MaxP95Ms = f(400) }},
		b:    askVariant{goals: func(g *engine.Goals) { g.MaxP95Ms = f(100) }},
		says: "The design answers in about 160 ms at the 95th percentile. That is fine against a 400 ms target and a failure against a 100 ms one, which would mean a different design with fewer hops. The target comes from the requirement, so it has to be asked.",
	}
}

func flipBudget() *askFlip {
	return &askFlip{
		a:    askVariant{goals: func(g *engine.Goals) { g.MaxCostPerHour = f(2.0) }},
		b:    askVariant{goals: func(g *engine.Goals) { g.MaxCostPerHour = f(0.8) }},
		says: "The design costs about $1.30 an hour: affordable with $2.00 to spend and out of reach at $0.80, where you would run fewer servers and accept less headroom. Spend is a requirement like any other.",
	}
}

const noEffect = "Good to know, but it does not change how big the system has to be."

// askPrompt lays out a prompt: the two decisive questions go at the given places and the others fill the rest in order.
func askPrompt(id, title, summary, prompt string, at [2]int, decisive [2]AskOption, others []AskOption) {
	asks := make([]AskOption, 0, 8)
	next := 0
	for i := 0; i < 8; i++ {
		switch i {
		case at[0]:
			asks = append(asks, decisive[0])
		case at[1]:
			asks = append(asks, decisive[1])
		default:
			asks = append(asks, others[next])
			next++
		}
	}
	registerQuestion(&Question{ID: id, Title: title, Mode: ModeAsk, Difficulty: "Medium", Summary: summary, Prompt: prompt, Asks: asks, Picks: 3})
}

func noise(text, why string) AskOption { return AskOption{Text: text, why: why} }
func key(text, why string, flip *askFlip) AskOption {
	return AskOption{Text: text, decisive: true, why: why, flip: flip}
}

func init() {
	const ask = "You may ask three questions before you start drawing. Which ones would change the design the most?"

	askPrompt("ask-photo-feed", "A photo feed: what would you ask?", "A one-line brief. Pick the questions that change the design.",
		"An interviewer says: \"Design a photo feed for a social app.\" That is all you are told. "+ask,
		[2]int{2, 5}, [2]AskOption{
			key("What share of the requests are people viewing photos, and what share are uploads?", "Mostly viewing means a cache can take nearly all the load. If many requests are uploads, every one reaches the database, which then has to be sized for them.", flipReads()),
			key("How many people use it at the busiest time of day?", "You build for the peak, not the average. Ten times the users at the busiest moment means roughly ten times the servers.", flipPeak()),
		}, []AskOption{
			noise("Which cloud provider will it run on?", "The same design works on any of them. "+noEffect),
			noise("What programming language will the team use?", "A choice for the team to make later. "+noEffect),
			noise("Should there be a dark mode?", "A feature for the front end. "+noEffect),
			noise("How many engineers will build it?", "It shapes the schedule, not the size of the system."),
			noise("What will the app be called?", noEffect),
			noise("Will there also be a mobile app?", "The mobile app calls the same service, so the load is the same."),
		})

	askPrompt("ask-link-shortener", "A link shortener: what would you ask?", "Two questions settle the cache and the speed target.",
		"An interviewer says: \"Design a link shortener.\" Nothing more. "+ask,
		[2]int{1, 6}, [2]AskOption{
			key("Do a few links get most of the clicks, or are clicks spread evenly over all of them?", "If a few links take most of the clicks, a small cache holds them and the database is barely touched. If clicks are spread over millions of links, the cache rarely helps.", flipSkew()),
			key("How quickly must a click be redirected?", "A target of 400 ms and one of 100 ms lead to different designs. Without the number you cannot tell whether the design is fast enough.", flipLatency()),
		}, []AskOption{
			noise("Which domain name will the short links use?", noEffect),
			noise("Should the owner be able to pick their own short name?", "A feature to design later. "+noEffect),
			noise("What will the admin page look like?", noEffect),
			noise("Which database vendor does the company already pay for?", "A preference to weigh later, not a requirement that changes the size."),
			noise("Should expired links show a friendly page?", "A product detail. "+noEffect),
			noise("How many people will work on it?", "It shapes the schedule, not the size of the system."),
		})

	askPrompt("ask-breaking-news", "A news site: what would you ask?", "How big the spike is, and what you may spend.",
		"An interviewer says: \"Design the article pages for a news site.\" That is the whole brief. "+ask,
		[2]int{3, 7}, [2]AskOption{
			key("When a big story breaks, how many times busier does the site get than on a normal day?", "A site that is quiet most of the day and ten times busier for an hour needs headroom or a way to add servers quickly. The spike, not the average, sets the size.", flipPeak()),
			key("What is the monthly budget for running it?", "Money is a requirement. The same design is affordable at one budget and impossible at another, and then you would trade headroom for cost.", flipBudget()),
		}, []AskOption{
			noise("Which content management system do the editors use?", "It matters for publishing, not for how many readers the site must serve."),
			noise("Do we need an RSS feed?", "A feature. "+noEffect),
			noise("Should readers be able to comment?", "Comments are a separate feature that can be designed later. "+noEffect),
			noise("What font should article pages use?", noEffect),
			noise("Which team will own the site?", noEffect),
			noise("What is the site called?", noEffect),
		})

	askPrompt("ask-chat-history", "Chat history: what would you ask?", "How often old messages are read, and how fast they must load.",
		"An interviewer says: \"Design message history for a chat app.\" Nothing else. "+ask,
		[2]int{0, 5}, [2]AskOption{
			key("How often do people read old messages compared with sending new ones?", "If most of the traffic is reading, storing and caching for reads is the job. If people mostly send, the write path is the bottleneck.", flipReads()),
			key("How fast must a conversation open?", "A target in tens of milliseconds forces a different design from one in hundreds. The number is a requirement, not something to guess.", flipLatency()),
		}, []AskOption{
			noise("Should messages support emoji reactions?", "A feature to design later. "+noEffect),
			noise("What colour are the message bubbles?", noEffect),
			noise("Which languages must the app support?", noEffect),
			noise("Is there a desktop client as well?", "It calls the same service. "+noEffect),
			noise("How long will the project take?", "It shapes the schedule, not the size of the system."),
			noise("Will there be stickers?", "A feature. "+noEffect),
		})

	askPrompt("ask-ticket-sale", "A ticket sale: what would you ask?", "How many arrive at once, and whether they browse or buy.",
		"An interviewer says: \"Design the website that sells tickets for a stadium concert.\" "+ask,
		[2]int{4, 2}, [2]AskOption{
			key("How many people will arrive in the first minute of the sale?", "The first minute is the whole problem: the system is idle for weeks and then takes the entire load at once. Its size decides how many servers must already be running.", flipPeak()),
			key("Do most visitors just browse seats, or do most buy straight away?", "Browsing is reads and a cache helps. Buying is writes, which no cache absorbs, and the database must commit every one.", flipReads()),
		}, []AskOption{
			noise("Which payment company will you use?", "It changes an integration, not how big the system must be."),
			noise("Should tickets be sent as PDFs?", noEffect),
			noise("What currency are prices shown in?", noEffect),
			noise("Will there be a loyalty scheme?", "A feature to design later. "+noEffect),
			noise("What time does the box office close?", noEffect),
			noise("Which venues are included?", noEffect),
		})

	askPrompt("ask-leaderboard", "A game leaderboard: what would you ask?", "Whether attention is concentrated, and what it may cost.",
		"An interviewer says: \"Design the leaderboard for a mobile game.\" That is all. "+ask,
		[2]int{6, 1}, [2]AskOption{
			key("Is most of the attention on a few top players, or spread over everyone?", "If everyone looks at the same top few, a small cache serves nearly every view. If each player looks up their own rank, the requests are spread over every player and a cache helps little.", flipSkew()),
			key("What can you spend each month on running it?", "Money limits how many servers you can run. A tight budget means accepting less headroom, which changes what you build.", flipBudget()),
		}, []AskOption{
			noise("Which game genre is it for?", noEffect),
			noise("Should players have avatars?", "A feature for the front end. "+noEffect),
			noise("Which game engine does the studio use?", noEffect),
			noise("Will there be seasons that reset the scores?", "A feature to design later. "+noEffect),
			noise("Is there a friends-only view?", "A feature to design later. "+noEffect),
			noise("How many developers are on the team?", "It shapes the schedule, not the size of the system."),
		})
}
