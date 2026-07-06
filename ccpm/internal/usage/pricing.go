package usage

import "strings"

// ModelPrice is the public Anthropic API list price for one model, in US dollars
// per 1,000,000 tokens, split by token kind. Cache-write is the 5-minute
// ephemeral rate (1.25x input) and cache-read is the cheap re-read rate (0.1x
// input) — the standard Claude pricing shape.
type ModelPrice struct {
	Input      float64
	Output     float64
	CacheWrite float64
	CacheRead  float64
}

// Cost returns the dollar cost of t at this price.
func (p ModelPrice) Cost(t Tokens) float64 {
	return float64(t.Input)/1e6*p.Input +
		float64(t.Output)/1e6*p.Output +
		float64(t.CacheCreation)/1e6*p.CacheWrite +
		float64(t.CacheRead)/1e6*p.CacheRead
}

// priceTable maps a model-family keyword to its list price. Model strings look
// like "claude-opus-4-8", "claude-sonnet-4-6", "claude-haiku-4-5-20251001", so
// we match by family substring. These are API-equivalent list prices (USD/1M)
// and are a best-effort estimate — a subscription's real cost is unrelated. Keep
// this current with anthropic.com/pricing.
var priceTable = []struct {
	keyword string
	price   ModelPrice
}{
	{"opus", ModelPrice{Input: 15, Output: 75, CacheWrite: 18.75, CacheRead: 1.5}},
	{"sonnet", ModelPrice{Input: 3, Output: 15, CacheWrite: 3.75, CacheRead: 0.3}},
	{"haiku", ModelPrice{Input: 0.8, Output: 4, CacheWrite: 1, CacheRead: 0.08}},
}

// PriceFor returns the list price for a model, and whether it was recognised.
func PriceFor(model string) (ModelPrice, bool) {
	m := strings.ToLower(model)
	for _, e := range priceTable {
		if strings.Contains(m, e.keyword) {
			return e.price, true
		}
	}
	return ModelPrice{}, false
}

// CostFor returns the estimated cost of t under model's list price (0 for an
// unrecognised model).
func CostFor(model string, t Tokens) float64 {
	p, ok := PriceFor(model)
	if !ok {
		return 0
	}
	return p.Cost(t)
}
