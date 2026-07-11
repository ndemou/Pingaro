package probe

import "context"

type Prober interface {
	Probe(context.Context, Request) Outcome
}
