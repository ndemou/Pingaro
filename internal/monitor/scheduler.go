package monitor

import (
	"context"
	"math"
	"strings"
	"time"

	"pingaro/internal/probe"
)

const DefaultReplyTimeout = 500 * time.Millisecond

type Group struct {
	ID      GroupID
	Name    string
	Targets []string
}

type SchedulerConfig struct {
	Clock                Clock
	Prober               probe.Prober
	SessionID            uint64
	Interval             time.Duration
	ReplyTimeout         time.Duration
	PerTargetOutstanding int
	GlobalOutstanding    int
}

type Scheduler struct {
	clock                Clock
	prober               probe.Prober
	sessionID            uint64
	interval             time.Duration
	replyTimeout         time.Duration
	perTargetOutstanding int
	globalOutstanding    int
	nextRequestID        uint64
}

type BatchResult struct {
	SessionID uint64
	GroupID   GroupID
	GroupName string
	SentAt    time.Time
	Deadline  time.Time
	Kind      probe.OutcomeKind
	RTT       time.Duration
	Warning   string
	Outcomes  []probe.Outcome
}

type probeCompletion struct {
	outcome     probe.Outcome
	completedAt time.Time
}

type batchState struct {
	sessionID uint64
	groupID   GroupID
	groupName string
	sentAt    time.Time
	deadline  time.Time
	pending   int
	sent      int
	finalized bool
	bestReply *probe.Outcome
	requests  []uint64
	outcomes  []probe.Outcome
}

func NewScheduler(cfg SchedulerConfig) *Scheduler {
	clock := cfg.Clock
	if clock == nil {
		clock = RealClock{}
	}
	replyTimeout := cfg.ReplyTimeout
	if replyTimeout <= 0 {
		replyTimeout = DefaultReplyTimeout
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Second
	}
	perTarget := cfg.PerTargetOutstanding
	if perTarget <= 0 {
		perTarget = DefaultPerTargetOutstandingLimit(replyTimeout, interval)
	}
	global := cfg.GlobalOutstanding
	if global <= 0 {
		global = math.MaxInt
	}
	return &Scheduler{
		clock:                clock,
		prober:               cfg.Prober,
		sessionID:            cfg.SessionID,
		interval:             interval,
		replyTimeout:         replyTimeout,
		perTargetOutstanding: perTarget,
		globalOutstanding:    global,
	}
}

func DefaultPerTargetOutstandingLimit(timeout, interval time.Duration) int {
	if interval <= 0 {
		interval = time.Second
	}
	if timeout <= 0 {
		timeout = DefaultReplyTimeout
	}
	return int(math.Ceil(float64(timeout)/float64(interval))) + 2
}

func DefaultGlobalOutstandingLimit(targetCount, perTarget int) int {
	if targetCount < 1 {
		targetCount = 1
	}
	if perTarget < 1 {
		perTarget = 1
	}
	return minInt(targetCount*perTarget, 2048)
}

func (s *Scheduler) Run(ctx context.Context, groups []Group, emit func(BatchResult)) {
	if s.prober == nil || len(groups) == 0 {
		return
	}

	completions := make(chan probeCompletion, minInt(maxInt(1, s.globalOutstanding), 4096))
	deadlines := make(chan *batchState, 1024)
	batches := map[uint64]*batchState{}
	perTargetOutstanding := map[string]int{}
	globalOutstanding := 0

	next := s.clock.Now()
	s.emitSlot(ctx, groups, next, batches, completions, deadlines, perTargetOutstanding, &globalOutstanding, emit)
	next = next.Add(s.interval)

	timer := s.clock.NewTimer(timeUntil(s.clock, next))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C():
			now := s.clock.Now()
			if now.Sub(next) >= s.interval {
				for !next.After(now) {
					next = next.Add(s.interval)
				}
			} else {
				s.emitSlot(ctx, groups, next, batches, completions, deadlines, perTargetOutstanding, &globalOutstanding, emit)
				next = next.Add(s.interval)
			}
			timer.Reset(timeUntil(s.clock, next))
		case completion := <-completions:
			req := completion.outcome.Request()
			if req.SessionID != s.sessionID {
				continue
			}
			if perTargetOutstanding[req.Target] > 0 {
				perTargetOutstanding[req.Target]--
			}
			if globalOutstanding > 0 {
				globalOutstanding--
			}
			batch := batches[req.ID]
			if batch == nil || batch.finalized {
				continue
			}
			batch.pending--
			batch.outcomes = append(batch.outcomes, completion.outcome)
			if isOnTimeReply(completion.outcome, completion.completedAt, batch.deadline, s.replyTimeout) {
				outcome := completion.outcome
				if batch.bestReply == nil || replyRTT(outcome) < replyRTT(*batch.bestReply) {
					batch.bestReply = &outcome
				}
			}
			if batch.pending == 0 {
				s.finalizeBatch(batch, batches, emit)
			}
		case batch := <-deadlines:
			if batch.sessionID != s.sessionID || batch.finalized {
				continue
			}
			s.finalizeBatch(batch, batches, emit)
		}
	}
}

func (s *Scheduler) emitSlot(ctx context.Context, groups []Group, sentAt time.Time, batches map[uint64]*batchState, completions chan<- probeCompletion, deadlines chan<- *batchState, perTargetOutstanding map[string]int, globalOutstanding *int, emit func(BatchResult)) {
	deadline := sentAt.Add(s.replyTimeout)
	for _, group := range groups {
		batch := &batchState{
			sessionID: s.sessionID,
			groupID:   group.ID,
			groupName: group.Name,
			sentAt:    sentAt,
			deadline:  deadline,
		}
		for _, target := range group.Targets {
			req := probe.Request{
				ID:        s.nextRequestID + 1,
				SessionID: s.sessionID,
				GroupID:   uint8(group.ID),
				Target:    target,
				SentAt:    sentAt,
				Deadline:  deadline,
			}
			s.nextRequestID = req.ID
			if perTargetOutstanding[target] >= s.perTargetOutstanding || *globalOutstanding >= s.globalOutstanding {
				batch.outcomes = append(batch.outcomes, probe.NewNotSent(req, "outstanding request limit reached"))
				continue
			}
			batch.sent++
			batch.pending++
			batch.requests = append(batch.requests, req.ID)
			batches[req.ID] = batch
			perTargetOutstanding[target]++
			*globalOutstanding++
			go func() {
				outcome := s.prober.Probe(ctx, req)
				completion := probeCompletion{outcome: outcome, completedAt: s.clock.Now()}
				select {
				case completions <- completion:
				case <-ctx.Done():
				}
			}()
		}
		if batch.pending == 0 {
			s.finalizeBatch(batch, batches, emit)
			continue
		}
		go func() {
			timer := s.clock.NewTimer(timeUntil(s.clock, deadline))
			defer timer.Stop()
			select {
			case <-timer.C():
				select {
				case deadlines <- batch:
				case <-ctx.Done():
				}
			case <-ctx.Done():
			}
		}()
	}
}

func (s *Scheduler) finalizeBatch(batch *batchState, batches map[uint64]*batchState, emit func(BatchResult)) {
	if batch.finalized {
		return
	}
	batch.finalized = true
	for _, requestID := range batch.requests {
		delete(batches, requestID)
	}
	result := BatchResult{
		SessionID: batch.sessionID,
		GroupID:   batch.groupID,
		GroupName: batch.groupName,
		SentAt:    batch.sentAt,
		Deadline:  batch.deadline,
		Outcomes:  append([]probe.Outcome(nil), batch.outcomes...),
	}
	if batch.bestReply != nil {
		result.Kind = probe.OutcomeReply
		result.RTT = replyRTT(*batch.bestReply)
		emit(result)
		return
	}
	result.Kind = classifyBatch(batch)
	result.Warning = batchWarning(batch.outcomes)
	emit(result)
}

func classifyBatch(batch *batchState) probe.OutcomeKind {
	if batch.sent == 0 {
		return probe.OutcomeNotSent
	}
	for _, outcome := range batch.outcomes {
		if outcome.Kind().CountsAsNetworkLoss() {
			return outcome.Kind()
		}
	}
	for _, outcome := range batch.outcomes {
		if outcome.Kind() != probe.OutcomeReply {
			return outcome.Kind()
		}
	}
	return probe.OutcomeTimeout
}

func batchWarning(outcomes []probe.Outcome) string {
	warnings := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		if detail := outcome.Detail(); detail != "" {
			warnings = append(warnings, outcome.Request().Target+": "+detail)
		}
	}
	return strings.Join(warnings, " | ")
}

func isOnTimeReply(outcome probe.Outcome, completedAt time.Time, deadline time.Time, replyTimeout time.Duration) bool {
	if outcome.Kind() != probe.OutcomeReply || completedAt.After(deadline) {
		return false
	}
	rtt, ok := outcome.RTT()
	return ok && rtt <= replyTimeout
}

func replyRTT(outcome probe.Outcome) time.Duration {
	rtt, _ := outcome.RTT()
	return rtt
}

func timeUntil(clock Clock, at time.Time) time.Duration {
	d := at.Sub(clock.Now())
	if d < 0 {
		return 0
	}
	return d
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
