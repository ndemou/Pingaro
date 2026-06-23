package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

const lostRTT = 9999

var (
	reTime   = regexp.MustCompile(`time[=<]([0-9]+)ms`)
	reTarget = regexp.MustCompile(`Reply from ([^:]+):`)

	esc = "\x1b"

	colReset   = esc + "[0m"
	colTitle   = esc + "[38;2;84;255;255m"
	colH1      = esc + "[38;2;107;235;163m"
	colWarn    = esc + "[38;2;25;163;147m"
	colBad     = esc + "[38;2;255;0;0m"
	graphBase  = esc + "[38;2;243;151;214m" + esc + "[48;2;14;70;70m"
	graphLow   = esc + "[38;2;107;235;163m" + esc + "[48;2;14;70;70m"
	graphHi    = esc + "[38;2;255;0;0m" + esc + "[48;2;243;151;214m"
	graphEmpty = esc + "[38;2;0;0;0m" + esc + "[48;2;14;70;70m"
)

type config struct {
	target             string
	targets            []string
	title              string
	graphMax           float64
	graphMin           float64
	pingsPerSec        int
	histBucketsCount   int
	aggregationSeconds int
	histSamples        int
	debugMode          bool
	highResFont        int
	updateScreenEvery  float64
	barGraphSamples    int
}

type highResFlag struct {
	value int
}

func (h *highResFlag) String() string {
	return strconv.Itoa(h.value)
}

func (h *highResFlag) Set(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "-1", "auto":
		h.value = -1
	case "0", "false", "$false", "low":
		h.value = 0
	case "1", "true", "$true", "high":
		h.value = 1
	default:
		return fmt.Errorf("expected -1/0/1, auto/low/high, or true/false")
	}
	return nil
}

type pingResult struct {
	sentAt      time.Time
	rtt         int
	destination string
	status      string
	warning     string
}

type renderState struct {
	cfg         config
	started     time.Time
	lastAgg     time.Time
	lastDraw    time.Time
	screen      string
	screenFile  string
	pingrecFile string

	rtts      []int
	p95s      []float64
	jitters   []float64
	losses    []float64
	toSave    []int
	allCount  int
	lostCount int
	minRTT    int
	maxRTT    int

	showHistogram bool
	showRecent    bool
	showLoss      bool
	showJitter    bool
	highRes       bool
	status        string
}

type barChars struct {
	hCount int
	hParts []string
	hFull  string
	vCount int
	vParts []string
}

func main() {
	cfg := parseFlags()
	if cfg.pingsPerSec < 1 {
		cfg.pingsPerSec = 1
	}
	if cfg.aggregationSeconds < 1 {
		cfg.aggregationSeconds = 120
	}
	if cfg.histSamples < 1 {
		cfg.histSamples = max(100, cfg.pingsPerSec*60)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ts := time.Now().Format("2006-01-02_15.04.05")
	tmp := os.TempDir()
	state := &renderState{
		cfg:           cfg,
		started:       time.Now(),
		lastAgg:       time.Now(),
		lastDraw:      time.Now().Add(-time.Hour),
		screenFile:    filepath.Join(tmp, "ops."+ts+".screen"),
		pingrecFile:   filepath.Join(tmp, "ops."+ts+".pingrec"),
		minRTT:        math.MaxInt,
		showHistogram: true,
		showRecent:    true,
		showLoss:      true,
		showJitter:    true,
		highRes:       cfg.highResFont != 0,
	}
	if cfg.highResFont < 0 {
		state.highRes = true
	}
	if cfg.title == "" {
		if len(cfg.targets) == 0 {
			cfg.title = "Internet hosts"
		} else {
			cfg.title = strings.Join(cfg.targets, ", ")
		}
		state.cfg.title = cfg.title
	}
	_ = os.WriteFile(state.pingrecFile, []byte(fmt.Sprintf("pingrec-v1,%s,%d pings/sec\n", ts, cfg.pingsPerSec)), 0644)

	keys := startKeyboard(ctx, stop, state)
	results := make(chan pingResult, 32)
	var wg sync.WaitGroup
	if len(cfg.targets) == 0 {
		wg.Add(1)
		go internetLoop(ctx, &wg, results)
	} else {
		wg.Add(1)
		go targetLoop(ctx, &wg, cfg.targets, cfg.pingsPerSec, results)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	clearScreen()
	for {
		select {
		case <-ctx.Done():
			fmt.Print(colReset + "\n")
			return
		case k := <-keys:
			handleKey(k, state)
		case r, ok := <-results:
			if !ok {
				return
			}
			state.accept(r)
			state.maybeAggregate()
			state.maybeRender()
		case <-time.After(time.Duration(cfg.updateScreenEvery*1000) * time.Millisecond):
			state.maybeRender()
		}
	}
}

func parseFlags() config {
	cfg := config{}
	highRes := highResFlag{value: -1}
	flag.StringVar(&cfg.target, "Target", "", "host name or IP address to probe")
	flag.StringVar(&cfg.title, "Title", "", "custom title shown at the top of the screen")
	flag.Float64Var(&cfg.graphMax, "GraphMax", -1, "upper Y-axis limit for RTT graphs")
	flag.Float64Var(&cfg.graphMin, "GraphMin", -1, "lower Y-axis limit for RTT graphs")
	flag.IntVar(&cfg.pingsPerSec, "PingsPerSec", 1, "pings per second for a specific target")
	flag.IntVar(&cfg.histBucketsCount, "HistBucketsCount", 10, "number of RTT histogram buckets")
	flag.IntVar(&cfg.histBucketsCount, "BucketsCount", 10, "alias for -HistBucketsCount")
	flag.IntVar(&cfg.aggregationSeconds, "AggregationSeconds", 120, "seconds per aggregation period")
	flag.IntVar(&cfg.histSamples, "HistSamples", -1, "samples to include in the RTT histogram")
	flag.BoolVar(&cfg.debugMode, "DebugMode", false, "print diagnostic details")
	flag.Var(&highRes, "HighResFont", "-1/auto, 0/false/low, or 1/true/high")
	flag.Float64Var(&cfg.updateScreenEvery, "UpdateScreenEvery", 1, "screen refresh period in seconds")
	flag.IntVar(&cfg.barGraphSamples, "BarGraphSamples", -1, "number of recent samples to show in scrolling bar graphs")
	_ = flag.CommandLine.Parse(reorderArgs(os.Args[1:]))

	cfg.targets = parseTargets(cfg.target, flag.Args())
	if len(cfg.targets) > 4 {
		fmt.Fprintf(os.Stderr, "error: specific target mode accepts at most 4 targets, got %d: %s\n", len(cfg.targets), strings.Join(cfg.targets, ", "))
		os.Exit(2)
	}
	cfg.highResFont = highRes.value
	return cfg
}

func reorderArgs(args []string) []string {
	valueFlags := map[string]bool{
		"Target":             true,
		"Title":              true,
		"GraphMax":           true,
		"GraphMin":           true,
		"PingsPerSec":        true,
		"HistBucketsCount":   true,
		"BucketsCount":       true,
		"AggregationSeconds": true,
		"HistSamples":        true,
		"HighResFont":        true,
		"UpdateScreenEvery":  true,
		"BarGraphSamples":    true,
	}
	boolFlags := map[string]bool{
		"DebugMode": true,
	}
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if idx := strings.IndexRune(name, '='); idx >= 0 {
			name = name[:idx]
		}
		if valueFlags[name] {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		if boolFlags[name] {
			flags = append(flags, arg)
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func parseTargets(targetFlag string, positional []string) []string {
	fields := make([]string, 0, 4)
	add := func(value string) {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}) {
			part = strings.TrimSpace(part)
			if part != "" {
				fields = append(fields, part)
			}
		}
	}
	add(targetFlag)
	for _, arg := range positional {
		add(arg)
	}
	return fields
}

func targetLoop(ctx context.Context, wg *sync.WaitGroup, targets []string, pingsPerSec int, out chan<- pingResult) {
	defer wg.Done()
	period := time.Second / time.Duration(pingsPerSec)
	if period < 50*time.Millisecond {
		period = 50 * time.Millisecond
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			out <- pingBatch(ctx, targets, "Targets", t)
		}
	}
}

func internetLoop(ctx context.Context, wg *sync.WaitGroup, out chan<- pingResult) {
	defer wg.Done()
	hosts := []string{"1.1.1.1", "1.1.1.2", "8.8.8.8", "8.8.4.4"}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			out <- pingBatch(ctx, hosts, "Internet", t)
		}
	}
}

func pingBatch(ctx context.Context, hosts []string, destination string, sentAt time.Time) pingResult {
	var batchWG sync.WaitGroup
	ch := make(chan pingResult, len(hosts))
	for _, h := range hosts {
		h := h
		batchWG.Add(1)
		go func() {
			defer batchWG.Done()
			ch <- pingOnce(ctx, h, sentAt)
		}()
	}
	batchWG.Wait()
	close(ch)
	best := pingResult{sentAt: sentAt, rtt: lostRTT, destination: destination, status: "failure"}
	replies := 0
	warnings := make([]string, 0, len(hosts))
	for r := range ch {
		if r.status == "Success" {
			replies++
			if r.rtt < best.rtt {
				best = r
				best.destination = destination
			}
		} else if r.warning != "" {
			warnings = append(warnings, r.destination+": "+r.warning)
		}
	}
	if replies > 0 {
		best.status = "Success"
		return best
	}
	if len(warnings) > 0 {
		best.warning = strings.Join(warnings, " | ")
	}
	return best
}

func pingOnce(ctx context.Context, host string, sentAt time.Time) pingResult {
	args := []string{"-n", "1", "-w", "1000", host}
	if runtime.GOOS != "windows" {
		args = []string{"-c", "1", "-W", "1", host}
	}
	cctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(cctx, "ping", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	text := buf.String()

	m := reTime.FindStringSubmatch(text)
	if len(m) == 2 {
		rtt, _ := strconv.Atoi(m[1])
		dest := host
		if dm := reTarget.FindStringSubmatch(text); len(dm) == 2 {
			dest = strings.TrimSpace(dm[1])
		}
		return pingResult{sentAt: sentAt, rtt: max(1, rtt), destination: dest, status: "Success"}
	}
	status := "TimeOut"
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		status = "PingFailed"
	}
	return pingResult{sentAt: sentAt, rtt: lostRTT, destination: host, status: status, warning: firstLine(text)}
}

func (s *renderState) accept(r pingResult) {
	if r.status == "Success" {
		if r.rtt < s.minRTT {
			s.minRTT = r.rtt
		}
		if r.rtt > s.maxRTT {
			s.maxRTT = r.rtt
		}
		s.rtts = append(s.rtts, r.rtt)
		s.toSave = append(s.toSave, r.rtt)
	} else {
		s.lostCount++
		s.rtts = append(s.rtts, lostRTT)
		s.toSave = append(s.toSave, -1)
	}
	if r.warning != "" {
		s.status = r.warning
	}
	s.allCount++
	keep := max(s.effectiveBars()+100, s.cfg.histSamples)
	if len(s.rtts) > keep {
		s.rtts = append([]int(nil), s.rtts[len(s.rtts)-keep:]...)
	}
}

func (s *renderState) maybeAggregate() {
	if time.Since(s.lastAgg) < time.Duration(s.cfg.aggregationSeconds)*time.Second || len(s.rtts) < 2 {
		return
	}
	s.lastAgg = time.Now()
	n := min(len(s.rtts), s.cfg.aggregationSeconds*max(1, s.cfg.pingsPerSec))
	window := s.rtts[len(s.rtts)-n:]
	ok := withoutLost(window)
	if len(ok) > 0 {
		s.p95s = append(s.p95s, statsInts(ok).p95)
	} else {
		s.p95s = append(s.p95s, 0)
	}
	s.jitters = append(s.jitters, p95Jitter(window))
	lost := 0
	for _, v := range window {
		if v == lostRTT {
			lost++
		}
	}
	s.losses = append(s.losses, float64(lost)*100/float64(len(window)))
	maxSlow := s.effectiveBars() + 100
	if len(s.p95s) > maxSlow {
		s.p95s = s.p95s[len(s.p95s)-maxSlow:]
		s.jitters = s.jitters[len(s.jitters)-maxSlow:]
		s.losses = s.losses[len(s.losses)-maxSlow:]
	}
	s.appendPingrec()
}

func (s *renderState) maybeRender() {
	if time.Since(s.lastDraw) < time.Duration(s.cfg.updateScreenEvery*1000)*time.Millisecond {
		return
	}
	s.lastDraw = time.Now()
	screen := s.render()
	s.screen = screen
	fmt.Print(esc + "[H" + screen)
	if len(s.p95s) > 0 {
		_ = os.WriteFile(s.screenFile, []byte(stripANSI(screen)), 0644)
	}
}

func (s *renderState) render() string {
	var b strings.Builder
	width := terminalWidth()
	secs := int(math.Ceil(time.Since(s.started).Seconds()))
	minRTT := s.minRTT
	if minRTT == math.MaxInt {
		minRTT = 0
	}
	rate := s.cfg.pingsPerSec
	if len(s.cfg.targets) == 0 {
		rate = 1
	}
	fmt.Fprintf(&b, "%s%s - %d pings, %d\", ~%dpings/s, min=%d, max=%dms, lost=%d%s\n",
		colH1, s.cfg.title, s.allCount, secs, rate, minRTT, s.maxRTT, s.lostCount, colReset)
	fmt.Fprintf(&b, "%s     %s%s\n", colWarn, s.status, colReset)

	if s.showRecent && len(s.rtts) > 0 {
		values := tailInts(s.rtts, s.effectiveBars())
		ok := withoutLost(values)
		st := stats{min: 0, p5: 0, median: 0, p95: 0, max: 0}
		if len(ok) > 0 {
			st = statsInts(ok)
		}
		yMin := stdNumLE(st.min)
		if yMin < 10 {
			yMin = 0
		}
		maxToShow := st.p95
		if st.max > st.p95*3 {
			maxToShow = st.p95 * 3
		} else if st.max > st.p95 && st.max <= st.p95*2 {
			maxToShow = st.p95 * 2
		}
		yMax := yAxisMax(st.min, maxToShow, yMin, 9)
		if yMax > 300 {
			yMax = 300
		}
		if s.cfg.graphMin != -1 {
			yMin = s.cfg.graphMin
		}
		if s.cfg.graphMax != -1 {
			yMax = s.cfg.graphMax
		}
		title := fmt.Sprintf("LAST RTTs, %d pings, min=%s, p95=%s, max=%s, last=<last> (ms) (Ctrl-R)",
			len(values), approx(st.min), approx(st.p95), approx(st.max))
		b.WriteString(renderBarGraph(intsToFloats(values), title, true, lostRTT, yMin, yMax, s.chars()))
	}

	if s.showHistogram && len(s.rtts) > 0 {
		values := tailInts(s.rtts, s.cfg.histSamples)
		ok := withoutLost(values)
		p95 := 0.0
		if len(ok) > 0 {
			p95 = statsInts(ok).p95
		}
		fmt.Fprintf(&b, "    %sRTT HISTOGRAM, last %d samples, p95=%s%s%sms (Ctrl-H)%s\n",
			colTitle, s.cfg.histSamples, colH1, approx(p95), colTitle, colReset)
		b.WriteString(renderHistogram(values, s.cfg, s.chars()))
	}

	if len(s.p95s) > 0 {
		period := fmt.Sprintf("per %.1f' for %.1f'", float64(s.cfg.aggregationSeconds)/60, float64(s.cfg.aggregationSeconds*len(s.p95s))/60)
		maxItems := max(1, width-6)
		values := tailFloats(s.p95s, maxItems)
		st := statsFloats(values)
		yMax := yAxisMax(st.min, st.max, 0, 10)
		if yMax > 300 {
			yMax = 300
		}
		if s.cfg.graphMax != -1 {
			yMax = s.cfg.graphMax
		}
		b.WriteString(renderBarGraph(values, "RTT 95th PERCENTILE, "+period+", min=<min>, p95=<p95>, max=<max>, last=<last> (ms)", true, lostRTT, 0, yMax, s.chars()))
		if s.showLoss {
			b.WriteString(renderBarGraph(tailFloats(s.losses, maxItems), "LOSS%, "+period+", min=<min>%, p95=<p95>%, max=<max>%, last=<last>% (Ctrl-L)", true, 100, 0, 6, s.chars()))
		}
		if s.showJitter {
			b.WriteString(renderBarGraph(tailFloats(s.jitters, maxItems), "ONE-WAY JITTER, "+period+", min=<min>, p95=<p95>, max=<max>, last=<last> (ms) (Ctrl-J)", true, -1, 0, 50, s.chars()))
		}
		fmt.Fprintf(&b, "%s     (Saving to %s)%s\n", colWarn, s.screenFile, colReset)
	} else {
		fmt.Fprintf(&b, "%sYou can use Ctrl-H,J,L,R to hide/show graphs and Ctrl-S to change font%s\n", colWarn, colReset)
		if !s.highRes {
			fmt.Fprintf(&b, "%sWe are using low resolution characters (Ctrl-S to try high resolution)%s\n", colWarn, colReset)
		}
	}
	return padLines(b.String(), width)
}

func (s *renderState) effectiveBars() int {
	if s.cfg.barGraphSamples > 0 {
		return s.cfg.barGraphSamples
	}
	return max(10, terminalWidth()-6)
}

func (s *renderState) chars() barChars {
	if s.highRes {
		return barChars{8, []string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}, "▉", 8, []string{"_", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}}
	}
	return barChars{3, []string{" ", "▌", "█"}, "█", 5, []string{"_", "‗", "₌", "▄", "◘", "█"}}
}

func renderBarGraph(yValues []float64, title string, showStats bool, special int, yMin, yMax float64, ch barChars) string {
	var b strings.Builder
	if len(yValues) == 0 {
		return ""
	}
	st := statsFloats(filteredSpecial(yValues, float64(special)))
	if showStats {
		title = strings.ReplaceAll(title, "<min>", approx(st.min))
		title = strings.ReplaceAll(title, "<median>", approx(st.median))
		title = strings.ReplaceAll(title, "<p5>", approx(st.p5))
		title = strings.ReplaceAll(title, "<p95>", approx(st.p95))
		title = strings.ReplaceAll(title, "<max>", approx(st.max))
		title = strings.ReplaceAll(title, "<last>", approx(yValues[len(yValues)-1]))
	}
	if yMax <= yMin {
		yMax = yMin + 1
	}
	labelWidth := len(fmt.Sprintf("%.0f", yMax))
	if l := len(fmt.Sprintf("%.0f", yMin)); l > labelWidth {
		labelWidth = l
	}
	top := fmt.Sprintf("%*.0f|%s", labelWidth, yMax, graphBase)
	mid := fmt.Sprintf("%*s|%s", labelWidth, "", graphBase)
	bot := fmt.Sprintf("%*.0f|%s", labelWidth, yMin, graphBase)
	yMaxAdj := yMax + 0.1
	step := (yMaxAdj - yMin) / float64(ch.vCount) / 3
	if step <= 0 {
		step = 1
	}
	for _, v := range yValues {
		if int(v) == special {
			top += colBad + "*" + graphBase
			mid += colBad + "*" + graphBase
			bot += colBad + "*" + graphBase
			continue
		}
		if v < yMin {
			top += graphEmpty + "_" + graphBase
			mid += graphEmpty + "_" + graphBase
			bot += graphLow + "▼" + graphBase
			continue
		}
		if v == yMin {
			top += graphEmpty + "_" + graphBase
			mid += graphEmpty + "_" + graphBase
			bot += "_"
			continue
		}
		q := int(math.Round((v - yMin) / step))
		if q > 3*ch.vCount {
			top += graphHi + "▲" + graphBase
			mid += "█"
			bot += "█"
		} else if q >= 2*ch.vCount {
			top += ch.vParts[min(q-2*ch.vCount, len(ch.vParts)-1)]
			mid += "█"
			bot += "█"
		} else if q >= ch.vCount {
			top += graphEmpty + "_" + graphBase
			mid += ch.vParts[min(q-ch.vCount, len(ch.vParts)-1)]
			bot += "█"
		} else {
			top += graphEmpty + "_" + graphBase
			mid += graphEmpty + "_" + graphBase
			bot += ch.vParts[min(max(0, q), len(ch.vParts)-1)]
		}
	}
	ticks := strings.Repeat("`         ", int(math.Ceil(float64(len(yValues))/10)))
	if rem := len(yValues) % 10; rem != 0 && len(ticks) >= 10-rem {
		ticks = ticks[10-rem:]
	}
	fmt.Fprintf(&b, "%s%*s %s%s\n", colTitle, labelWidth, "", title, colReset)
	fmt.Fprintf(&b, "%s%s\n%s%s\n%s%s\n", top, colReset, mid, colReset, bot, colReset)
	fmt.Fprintf(&b, "%s%*s %s%s\n", colTitle, labelWidth, "", ticks, colReset)
	return b.String()
}

func renderHistogram(values []int, cfg config, ch barChars) string {
	if len(values) == 0 {
		return ""
	}
	st := statsInts(values)
	yMin := int(st.min)
	if cfg.graphMin != -1 {
		yMin = int(cfg.graphMin)
	}
	yMax := int(math.Min(999, st.p95*1.1))
	if cfg.graphMax != -1 {
		yMax = int(cfg.graphMax)
	}
	if yMax < yMin+cfg.histBucketsCount {
		yMax = yMin + cfg.histBucketsCount
	}
	yMax = int(math.Ceil(float64(yMax-yMin)/float64(cfg.histBucketsCount)))*cfg.histBucketsCount + yMin

	buckets := make([]int, cfg.histBucketsCount+1)
	for _, ms := range values {
		if ms == lostRTT {
			buckets[cfg.histBucketsCount]++
			continue
		}
		norm := min(yMax-1, max(yMin, ms))
		bucket := int(math.Floor(float64(norm-yMin) / float64(yMax-yMin) * float64(cfg.histBucketsCount)))
		buckets[bucket]++
	}
	maxPerc := 0.0
	for _, c := range buckets {
		maxPerc = math.Max(maxPerc, 100*float64(c)/float64(len(values)))
	}
	if maxPerc == 0 {
		maxPerc = 1
	}
	charsFor100 := 28 / (maxPerc / 100)
	var b strings.Builder
	cumul := 0.0
	for i := 0; i < cfg.histBucketsCount; i++ {
		from := yMin + i*(yMax-yMin)/cfg.histBucketsCount
		to := yMin + (i+1)*(yMax-yMin)/cfg.histBucketsCount
		percent := math.Round(1000*float64(buckets[i])/float64(len(values))) / 10
		cumul = math.Min(100, math.Round((cumul+percent)*10)/10)
		fromStr := fmt.Sprintf("%3d", from)
		cumulStr := " Cumul"
		if i > 0 {
			cumulStr = fmt.Sprintf(" %3.0f%% ", cumul)
		}
		toStr := fmt.Sprintf("%3d", to)
		if i == cfg.histBucketsCount-1 {
			toStr = "MAX"
		}
		bar := percentToBar(percent, charsFor100, ch)
		fmt.Fprintf(&b, "%s...%s %s%4d%s %4.1f%%%s%5s%s%-28s%s\n",
			fromStr, toStr, colWarn, buckets[i], colReset, percent, colWarn, cumulStr, graphBase, bar, colReset)
	}
	fail := buckets[cfg.histBucketsCount]
	percent := math.Round(1000*float64(fail)/float64(len(values))) / 10
	color := colH1
	if percent > 0 {
		color = colBad
	}
	fmt.Fprintf(&b, "Failures: %s%4d %4.1f%%      %s%s\n", color, fail, percent, percentToBar(percent, charsFor100, ch), colReset)
	return b.String()
}

func percentToBar(percent, charsFor100 float64, ch barChars) string {
	length := percent / 100 * charsFor100
	full := int(math.Floor(length))
	remainder := int(math.Floor((length - math.Floor(length)) * float64(ch.hCount)))
	bar := strings.Repeat(ch.hFull, full)
	if remainder > 0 {
		bar += ch.hParts[min(remainder, len(ch.hParts)-1)]
	}
	if len([]rune(bar)) < 28 {
		bar += strings.Repeat(" ", 28-len([]rune(bar)))
	}
	return bar
}

func startKeyboard(ctx context.Context, stop context.CancelFunc, state *renderState) <-chan byte {
	keys := make(chan byte, 8)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil
	}
	go func() {
		<-ctx.Done()
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}()
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			if buf[0] == 3 {
				stop()
				return
			}
			keys <- buf[0]
			_ = state
		}
	}()
	return keys
}

func handleKey(k byte, s *renderState) {
	switch k {
	case 8:
		s.showHistogram = !s.showHistogram
		clearScreen()
	case 10:
		s.showJitter = !s.showJitter
		clearScreen()
	case 12:
		s.showLoss = !s.showLoss
		clearScreen()
	case 18:
		s.showRecent = !s.showRecent
		clearScreen()
	case 19:
		s.highRes = !s.highRes
	}
}

func (s *renderState) appendPingrec() {
	if len(s.toSave) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString(time.Now().Format("1504:"))
	for _, v := range s.toSave {
		b.WriteRune(rune(v + 34))
	}
	b.WriteByte('\n')
	f, err := os.OpenFile(s.pingrecFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err == nil {
		_, _ = f.WriteString(b.String())
		_ = f.Close()
	}
	s.toSave = nil
}

type stats struct {
	min, p5, median, p95, max float64
}

func statsInts(values []int) stats {
	f := make([]float64, 0, len(values))
	for _, v := range values {
		f = append(f, float64(v))
	}
	return statsFloats(f)
}

func statsFloats(values []float64) stats {
	if len(values) == 0 {
		return stats{}
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	p5 := max(0, int(float64(len(cp))*0.05)-1)
	p95 := max(0, int(float64(len(cp))*0.95)-1)
	return stats{min: cp[0], p5: cp[p5], median: cp[len(cp)/2], p95: cp[p95], max: cp[len(cp)-1]}
}

func p95Jitter(values []int) float64 {
	var jitters []float64
	prev := values[0]
	for _, v := range values[1:] {
		if v != lostRTT && prev != lostRTT {
			jitters = append(jitters, math.Round(math.Abs(float64(v-prev))/2))
		}
		prev = v
	}
	if len(jitters) == 0 {
		return 0
	}
	return statsFloats(jitters).p95
}

func stdNumLE(x float64) float64 {
	if x <= 0 {
		return 0
	}
	power := math.Floor(math.Log10(x + 1))
	candidate := math.Pow(10, power)
	if candidate > x {
		candidate /= 10
	}
	if candidate*5 <= x {
		candidate *= 5
	}
	if candidate*2 <= x {
		candidate *= 2
	}
	return candidate
}

func stdNumGE(x float64) float64 {
	if x <= 0 {
		return 1
	}
	power := math.Ceil(math.Log10(x + 1))
	candidate := math.Pow(10, power)
	if candidate*1.5 >= x && candidate*0.9 < x {
		return candidate * 1.5
	}
	if candidate*0.9 >= x && candidate*0.6 < x {
		return candidate * 0.9
	}
	if candidate*0.6 >= x && candidate*0.3 < x {
		return candidate * 0.6
	}
	if candidate*0.3 >= x && candidate*0.15 < x {
		return candidate * 0.3
	}
	return candidate * 0.15
}

func yAxisMax(minValue, maxValue, yMin, minRange float64) float64 {
	yMax := stdNumGE(math.Max(yMin+minRange, maxValue))
	if yMax == yMin {
		yMax = stdNumGE(yMax + 1)
	}
	_ = minValue
	return yMax
}

func approx(x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "???"
	}
	digits := 0
	ax := math.Abs(x)
	if ax != 0 && int(ax) == 0 {
		digits = int(math.Abs(math.Floor(math.Log10(ax)))) + 2
	} else {
		intDigits := len(fmt.Sprintf("%d", int(ax)))
		if intDigits >= 4 {
			digits = 0
		} else {
			digits = 4 - intDigits
		}
	}
	format := "%." + strconv.Itoa(max(0, digits)) + "f"
	out := fmt.Sprintf(format, x)
	return strings.TrimRight(strings.TrimRight(out, "0"), ".")
}

func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 20 {
		return 100
	}
	return w
}

func clearScreen() {
	fmt.Print(esc + "[2J" + esc + "[H")
}

func padLines(s string, width int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var b strings.Builder
	for _, line := range lines {
		visible := len([]rune(stripANSI(line)))
		b.WriteString(line)
		if visible < width-1 {
			b.WriteString(strings.Repeat(" ", width-1-visible))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m|\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func withoutLost(values []int) []int {
	out := make([]int, 0, len(values))
	for _, v := range values {
		if v != lostRTT {
			out = append(out, v)
		}
	}
	return out
}

func filteredSpecial(values []float64, special float64) []float64 {
	out := make([]float64, 0, len(values))
	for _, v := range values {
		if v != special {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return values
	}
	return out
}

func intsToFloats(values []int) []float64 {
	out := make([]float64, 0, len(values))
	for _, v := range values {
		out = append(out, float64(v))
	}
	return out
}

func tailInts(values []int, n int) []int {
	if len(values) <= n {
		return append([]int(nil), values...)
	}
	return append([]int(nil), values[len(values)-n:]...)
}

func tailFloats(values []float64, n int) []float64 {
	if len(values) <= n {
		return append([]float64(nil), values...)
	}
	return append([]float64(nil), values[len(values)-n:]...)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
