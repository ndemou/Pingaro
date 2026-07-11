package main

//go:generate go run github.com/akavel/rsrc@v0.10.2 -manifest pingaro.exe.manifest -ico assets/pingaro.ico -o rsrc.syso

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"pingaro/internal/history"
	"pingaro/internal/monitor"
	"pingaro/internal/probe"
	"pingaro/internal/profiles"
	"pingaro/internal/settings"
	"pingaro/internal/targets"
)

const historyLostRTT = 9999

const (
	appIconResourceID      = 2
	defaultInternetTargets = settings.DefaultInternetTargets
)

const (
	initialWindowWidth        = 1180
	initialWindowHeight       = 760
	minAutosaveHistorySamples = 10
	autosaveHistoryInterval   = 10 * time.Minute
)

var (
	appVersion   = "dev"
	appBuildTime = ""
)

const (
	rttChartHeight       = 150
	aggregateChartHeight = 145
	chartHeaderHeight    = 18
	headerChartGap       = 2
	chartMinHeight       = 0
	xAxisLabelWidth      = 64
)

const (
	aggregateMinPixelsPerSample       = 6
	aggregatePreferredPixelsPerSample = aggregateMinPixelsPerSample * 4
	realtimeBarWidth                  = 4
	realtimeBarHighlightPadding       = 3
	realtimePixelsPerSample           = realtimeBarWidth + 2*realtimeBarHighlightPadding
	realtimeLossMarkerSize            = 10
	realtimeLossMarkerStrokeWidth     = 2
)

var (
	rttYMaxBuckets    = []float64{4, 8, 16, 32, 64, 128, 256, 512}
	lossYMaxBuckets   = []float64{2, 4, 8, 16, 32}
	jitterYMaxBuckets = []float64{8, 16, 32, 64, 128, 256}
)

type pingResult struct {
	sentAt      time.Time
	rtt         int
	destination string
	kind        probe.OutcomeKind
	warning     string
}

type sampleEvent struct {
	at          time.Time
	groupID     monitor.GroupID
	rtt         int
	replied     bool
	lost        bool
	targetLabel string
	minRTT      int
	maxRTT      int
	total       int
	lostTotal   int
	lossPercent float64
	p95         float64
	jitterP95   float64
	windowLoss  float64
	warning     string
	aggregate   *aggregatePoint
	period      time.Duration
}

type historyFile = history.File

type historySample = history.Sample

type historyAggregate = history.Aggregate

type streamState struct {
	groupID       monitor.GroupID
	targetLabel   string
	aggSeconds    int
	pingsPerBatch int
	observations  []observation
	total         int
	lostTotal     int
	minRTT        int
	maxRTT        int
	lastAgg       time.Time
}

type aggregatePoint struct {
	groupID   monitor.GroupID
	groupName string
	at        time.Time
	p95       float64
	loss      float64
	jitterP95 float64
}

type chartPoint struct {
	at         time.Time
	value      float64
	hasValue   bool
	lost       bool
	groupIndex int
	groupName  string
	color      walk.Color
	severity   int
}

type chartPlotPoint struct {
	x float64
	y float64
}

type realtimeBarSample struct {
	at     time.Time
	points []chartPoint
}

type realtimeBarSegment struct {
	color     walk.Color
	fromValue float64
	toValue   float64
}

type observation struct {
	rtt     int
	replied bool
	lost    bool
}

type timeAxisTick struct {
	at    time.Time
	label string
}

type connectionIssue struct {
	at       time.Time
	severity int
}

type connectionIssueBucket struct {
	severity int
	count    int
	first    time.Time
	last     time.Time
}

type lastItem struct {
	Text  string
	Color walk.Color
}

type targetGroup struct {
	ID      monitor.GroupID
	Name    string
	Targets []string
}

type savedConfig = settings.Config

type savedGroup = settings.Group

type useProfile = profiles.Profile

type app struct {
	*walk.MainWindow

	groupNames   [3]*walk.LineEdit
	groupTargets [3]*walk.LineEdit
	useChecks    [6]*walk.CheckBox
	pps          *walk.LineEdit
	agg          *walk.LineEdit
	startButton  *walk.PushButton
	stopButton   *walk.PushButton
	rttChart     *walk.CustomWidget
	p95Chart     *walk.CustomWidget
	lossChart    *walk.CustomWidget
	jitterChart  *walk.CustomWidget
	currentLabel *walk.Label
	summaryPanel *walk.Composite
	summaryLabel *walk.TextLabel

	cancel            context.CancelFunc
	samples           []sampleEvent
	aggregates        []aggregatePoint
	pendingSamples    []historySample
	pendingAggregates []historyAggregate
	period            time.Duration
	colors            []walk.Color
	settings          savedConfig
	lastHistorySave   time.Time
	autosavePath      string
	historyViewEnd    time.Time
	startedAt         time.Time
	sessionID         uint64
	sessionPings      int
	rttChartHeight    int
	p95ChartHeight    int
	lossChartHeight   int
	jitterChartHeight int
	summarySeverity   int
	aggregateEmptyAt  time.Time
}

var useProfiles = profiles.All()

func useTypes() []string {
	return profiles.Names()
}

func defaultUseTypes() []string {
	return profiles.DefaultUseTypes()
}

func normalizeUseType(value string) string {
	return profiles.NormalizeUseType(value)
}

func normalizeUseTypes(values []string, legacyValue string) []string {
	return profiles.NormalizeUseTypes(values, legacyValue)
}

func useTypeSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func profileFor(name string) (useProfile, bool) {
	return profiles.ForName(name)
}

func profileForUses(names []string) useProfile {
	return profiles.ForUses(names)
}

func usesShowJitter(names []string) bool {
	return profiles.UsesShowJitter(names)
}

func main() {
	a := &app{
		period:   120 * time.Second,
		colors:   defaultGroupColors(),
		settings: loadConfig(),
	}
	if err := a.run(); err != nil {
		log.Fatal(err)
	}
}

func buildInfoText() string {
	return formatBuildInfo(appVersion, appBuildTime, executableModTime())
}

func formatBuildInfo(version, buildTime string, fallbackBuildTime time.Time) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}
	return fmt.Sprintf("%s - built %s", version, formatBuildTime(buildTime, fallbackBuildTime))
}

func formatBuildTime(buildTime string, fallback time.Time) string {
	buildTime = strings.TrimSpace(buildTime)
	if buildTime != "" {
		if parsed, err := time.Parse(time.RFC3339, buildTime); err == nil {
			return parsed.Local().Format("2006-01-02 15:04:05")
		}
		return buildTime
	}
	if !fallback.IsZero() {
		return fallback.Local().Format("2006-01-02 15:04:05")
	}
	return "unknown"
}

func executableModTime() time.Time {
	path, err := os.Executable()
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func defaultGroupColors() []walk.Color {
	return []walk.Color{
		walk.RGB(0, 154, 222),
		walk.RGB(175, 89, 186),
		walk.RGB(255, 31, 91),
	}
}

func groupIDFromIndex(index int) monitor.GroupID {
	if index < 0 {
		return 0
	}
	if index > 255 {
		return monitor.GroupID(255)
	}
	return monitor.GroupID(index)
}

func (a *app) groupColor(id monitor.GroupID) walk.Color {
	idx := id.Index()
	if idx >= 0 && idx < len(a.colors) {
		return a.colors[idx]
	}
	return walk.RGB(80, 90, 100)
}

func loadConfig() savedConfig {
	return loadConfigFromPaths(configPath(), legacyConfigPath())
}

func loadConfigFromPaths(path, legacyPath string) savedConfig {
	return settings.LoadFromPaths(path, legacyPath)
}

func readSavedConfig(path string) (savedConfig, bool) {
	return settings.Read(path)
}

func normalizeLoadedConfig(cfg savedConfig) savedConfig {
	return settings.NormalizeLoaded(cfg)
}

func baseConfig() savedConfig {
	return settings.Base()
}

func defaultConfig() savedConfig {
	return settings.Default()
}

func defaultGroups() []savedGroup {
	return settings.DefaultGroups()
}

func normalizeSavedGroups(groups []savedGroup) []savedGroup {
	return settings.NormalizeGroups(groups)
}

func saveConfig(cfg savedConfig) {
	_ = writeConfigFile(configPath(), cfg)
}

func writeConfigFile(path string, cfg savedConfig) error {
	return settings.WriteFile(path, cfg)
}

func configPath() string {
	return appDataPath("settings.json")
}

func legacyConfigPath() string {
	return appDataPath("pingaro.json")
}

func defaultHistoryPath() string {
	return appDataPath("history.json")
}

func legacyHistoryPath() string {
	return appDataPath("pingaro-history.json")
}

func autosaveHistoryDir() string {
	return filepath.Dir(defaultHistoryPath())
}

func activeDefaultHistoryPath() string {
	path := defaultHistoryPath()
	_ = migrateLegacyHistoryFile(path, legacyHistoryPath())
	return path
}

func migrateLegacyHistoryFile(path, legacyPath string) error {
	return history.MigrateLegacyFile(path, legacyPath)
}

func createAutosaveHistoryFile(dir string, startedAt time.Time, pid int) (string, error) {
	return history.CreateAutosaveFile(dir, startedAt, pid)
}

func autosaveHistoryFilename(stem string, pid, suffix int) string {
	return history.AutosaveFilename(stem, pid, suffix)
}

func appDataPath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "Pingaro", name)
}

func defaultGateway() string {
	return targets.DefaultGateway()
}

func (a *app) run() error {
	mw := MainWindow{
		AssignTo: &a.MainWindow,
		Title:    "Pingaro - Long term network quality monitor",
		Icon:     appIconResourceID,
		MinSize:  Size{980, 650},
		Size:     Size{initialWindowWidth, initialWindowHeight},
		Layout:   VBox{MarginsZero: true},
		Children: []Widget{
			HSplitter{
				HandleWidth: 10,
				Children: []Widget{
					Composite{
						MinSize:       Size{180, 0},
						StretchFactor: 1,
						Layout:        VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
						Children: []Widget{
							Label{Text: "Target groups"},
							a.groupEditor(0),
							a.groupEditor(1),
							a.groupEditor(2),
							Label{Text: "Use profiles"},
							a.useTypeEditor(),
							Composite{
								Layout: VBox{MarginsZero: true, Spacing: 3},
								Children: []Widget{
									a.lineEditor("Measurements/sec", &a.pps, strconv.Itoa(max(1, a.settings.PPS))),
									a.lineEditor("Aggregation window (sec)", &a.agg, strconv.Itoa(max(3, a.settings.AggregationSeconds))),
								},
							},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true},
								Children: []Widget{
									PushButton{AssignTo: &a.startButton, Text: "Start", OnClicked: a.start},
									PushButton{AssignTo: &a.stopButton, Text: "Stop", Enabled: false, OnClicked: a.stop},
								},
							},
							Label{Text: "History"},
							Composite{
								Layout: Grid{Columns: 2, MarginsZero: true},
								Children: []Widget{
									PushButton{Text: "Save", OnClicked: a.saveHistoryDialog},
									PushButton{Text: "Load", OnClicked: a.loadHistoryDialog},
								},
							},
							VSpacer{},
							Label{AssignTo: &a.currentLabel, Text: "No measurements yet"},
							Composite{
								AssignTo: &a.summaryPanel,
								Layout:   VBox{Margins: Margins{Left: 6, Top: 4, Right: 6, Bottom: 4}},
								Visible:  false,
								Background: SolidColorBrush{
									Color: summaryBackgroundColor(0),
								},
								Children: []Widget{
									TextLabel{
										AssignTo:      &a.summaryLabel,
										Text:          "",
										MinSize:       Size{150, 0},
										Font:          Font{Family: "Segoe UI", PointSize: 9, Bold: true},
										Background:    TransparentBrush{},
										TextAlignment: AlignHNearVNear,
									},
								},
							},
							TextLabel{
								Text:          buildInfoText(),
								MinSize:       Size{150, 0},
								Font:          Font{Family: "Segoe UI", PointSize: 8},
								TextColor:     walk.RGB(120, 130, 140),
								Background:    TransparentBrush{},
								TextAlignment: AlignHNearVNear,
							},
						},
					},
					Composite{
						StretchFactor: 3,
						Layout:        VBox{Margins: Margins{Left: 4, Top: 4, Right: 4, Bottom: 4}, Spacing: 0},
						Children: []Widget{
							VSpacer{Size: 2},
							CustomWidget{AssignTo: &a.rttChart, MinSize: chartWidgetMinSize(), StretchFactor: rttChartHeight, InvalidatesOnResize: true, PaintPixels: a.paintRTT},
							CustomWidget{AssignTo: &a.p95Chart, MinSize: chartWidgetMinSize(), StretchFactor: aggregateChartHeight, InvalidatesOnResize: true, PaintPixels: a.paintP95},
							CustomWidget{AssignTo: &a.lossChart, MinSize: chartWidgetMinSize(), StretchFactor: aggregateChartHeight, InvalidatesOnResize: true, PaintPixels: a.paintLoss},
							CustomWidget{AssignTo: &a.jitterChart, MinSize: chartWidgetMinSize(), StretchFactor: aggregateChartHeight, InvalidatesOnResize: true, PaintPixels: a.paintJitter},
						},
					},
				},
			},
		},
	}
	if err := mw.Create(); err != nil {
		return err
	}
	a.attachInputPersistence()
	a.updateJitterChartVisibility()
	a.MainWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.saveInputs()
		_ = a.flushAutosaveHistory()
	})
	a.start()
	a.MainWindow.Run()
	return nil
}

func (a *app) groupEditor(index int) Widget {
	name, targets := "", ""
	if index < len(a.settings.Groups) {
		name = a.settings.Groups[index].Name
		targets = a.settings.Groups[index].Targets
	}
	return Composite{
		Layout: VBox{MarginsZero: true, Spacing: 0},
		Children: []Widget{
			a.groupLabel(index),
			a.lineEditor("Name", &a.groupNames[index], name),
			Label{Text: "Targets"},
			LineEdit{AssignTo: &a.groupTargets[index], Text: targets, MaxSize: Size{120, 0}},
		},
	}
}

func (a *app) lineEditor(label string, assignTo **walk.LineEdit, text string) Widget {
	return Composite{
		Layout: Grid{Columns: 2, MarginsZero: true, Spacing: 4},
		Children: []Widget{
			Label{Text: label},
			LineEdit{AssignTo: assignTo, Text: text, MaxSize: Size{70, 0}},
		},
	}
}

func (a *app) useTypeEditor() Widget {
	selected := useTypeSet(normalizeUseTypes(a.settings.UseTypes, a.settings.UseType))
	children := make([]Widget, 0, len(useProfiles))
	for i, profile := range useProfiles {
		children = append(children, CheckBox{
			AssignTo: &a.useChecks[i],
			Text:     profile.Name,
			Checked:  selected[profile.Name],
		})
	}
	return Composite{
		Layout:   VBox{MarginsZero: true, Spacing: 1},
		Children: children,
	}
}

func (a *app) groupLabel(index int) Widget {
	return Label{Text: fmt.Sprintf("Group %d", index+1)}
}

func (a *app) start() {
	a.stop()

	groups := a.targetGroups()
	if len(groups) == 0 {
		walk.MsgBox(a.MainWindow, "Pingaro", "Enter at least one target group.", walk.MsgBoxIconWarning)
		return
	}

	pps := clampInt(parseInt(a.pps.Text(), 1), 1, 20)
	aggSeconds := clampInt(parseInt(a.agg.Text(), 120), 3, 3600)
	a.saveCurrentConfig(groups, pps, aggSeconds)

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.samples = nil
	a.aggregates = nil
	a.pendingSamples = nil
	a.pendingAggregates = nil
	a.autosavePath = ""
	a.period = time.Duration(aggSeconds) * time.Second
	a.historyViewEnd = time.Time{}
	a.startedAt = time.Now()
	a.aggregateEmptyAt = a.startedAt
	a.sessionPings = 0
	a.lastHistorySave = time.Time{}
	a.setRunning(true)
	a.updateCurrentLabel()
	a.invalidateCharts()
	a.sessionID++
	sessionID := a.sessionID
	go a.pingLoop(ctx, groups, pps, aggSeconds, sessionID)
}

func (a *app) stop() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	_ = a.flushAutosaveHistory()
	a.setRunning(false)
}

func (a *app) setRunning(running bool) {
	a.startButton.SetEnabled(!running)
	a.stopButton.SetEnabled(running)
}

func (a *app) targetGroups() []targetGroup {
	groups := make([]targetGroup, 0, 3)
	gateway := ""
	gatewayLoaded := false
	for i := 0; i < 3; i++ {
		rawTargets := parseTargets(a.groupTargets[i].Text())
		if targetListNeedsGateway(rawTargets) && !gatewayLoaded {
			gateway = defaultGateway()
			gatewayLoaded = true
		}
		targets := resolveTargets(rawTargets, gateway)
		if len(targets) == 0 {
			continue
		}
		name := strings.TrimSpace(a.groupNames[i].Text())
		if name == "" {
			name = fmt.Sprintf("Group %d", i+1)
		}
		groups = append(groups, targetGroup{ID: groupIDFromIndex(i), Name: name, Targets: targets})
	}
	return groups
}

func (a *app) saveCurrentConfig(_ []targetGroup, pps, aggSeconds int) {
	cfg := savedConfig{PPS: pps, AggregationSeconds: aggSeconds, UseTypes: a.selectedUseTypes(), Groups: a.currentSavedGroups()}
	a.settings = cfg
	saveConfig(cfg)
}

func (a *app) saveInputs() {
	saveConfig(a.currentConfig())
}

func (a *app) attachInputPersistence() {
	for _, edit := range a.groupNames {
		if edit != nil {
			edit.TextChanged().Attach(a.saveInputs)
		}
	}
	for _, edit := range a.groupTargets {
		if edit != nil {
			edit.TextChanged().Attach(a.saveInputs)
		}
	}
	for _, edit := range []*walk.LineEdit{a.pps, a.agg} {
		if edit != nil {
			edit.TextChanged().Attach(a.saveInputs)
		}
	}
	for _, check := range a.useChecks {
		if check != nil {
			check.CheckedChanged().Attach(a.useTypesChanged)
		}
	}
}

func (a *app) useTypesChanged() {
	a.ensureUseSelection()
	a.saveInputs()
	a.updateJitterChartVisibility()
	a.updateCurrentLabel()
	a.invalidateCharts()
}

func (a *app) ensureUseSelection() {
	if len(a.selectedUseTypes()) > 0 {
		return
	}
	if a.useChecks[0] != nil {
		a.useChecks[0].SetChecked(true)
	}
}

func (a *app) selectedUseTypes() []string {
	if a.useChecks[0] == nil {
		return normalizeUseTypes(a.settings.UseTypes, a.settings.UseType)
	}
	var selected []string
	for i, profile := range useProfiles {
		if a.useChecks[i] != nil && a.useChecks[i].Checked() {
			selected = append(selected, profile.Name)
		}
	}
	return selected
}

func (a *app) selectedProfile() useProfile {
	return profileForUses(a.selectedUseTypes())
}

func (a *app) shouldShowJitterChart() bool {
	return usesShowJitter(a.selectedUseTypes())
}

func (a *app) updateJitterChartVisibility() {
	if a.jitterChart != nil {
		a.jitterChart.SetVisible(a.shouldShowJitterChart())
	}
}

func groupSummary(groups []targetGroup) string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	return strings.Join(names, ", ")
}

func (a *app) pingLoop(ctx context.Context, groups []targetGroup, pps, aggSeconds int, sessionID uint64) {
	period := time.Second / time.Duration(pps)
	if period < 50*time.Millisecond {
		period = 50 * time.Millisecond
	}
	states := make(map[monitor.GroupID]*streamState, len(groups))
	monitorGroups := make([]monitor.Group, 0, len(groups))
	targetCount := 0
	for _, g := range groups {
		states[g.ID] = &streamState{
			groupID:       g.ID,
			minRTT:        math.MaxInt,
			targetLabel:   g.Name,
			aggSeconds:    aggSeconds,
			pingsPerBatch: pps,
		}
		monitorGroups = append(monitorGroups, monitor.Group{ID: g.ID, Name: g.Name, Targets: g.Targets})
		targetCount += len(g.Targets)
	}
	perTargetLimit := monitor.DefaultPerTargetOutstandingLimit(monitor.DefaultReplyTimeout, period)
	scheduler := monitor.NewScheduler(monitor.SchedulerConfig{
		Clock:                monitor.RealClock{},
		Prober:               probe.NewICMPProber(),
		SessionID:            sessionID,
		Interval:             period,
		ReplyTimeout:         monitor.DefaultReplyTimeout,
		PerTargetOutstanding: perTargetLimit,
		GlobalOutstanding:    monitor.DefaultGlobalOutstandingLimit(targetCount, perTargetLimit),
	})
	scheduler.Run(ctx, monitorGroups, func(result monitor.BatchResult) {
		if ctx.Err() != nil {
			return
		}
		state := states[result.GroupID]
		if state == nil {
			return
		}
		ev := state.accept(pingResultFromBatchResult(result))
		a.Synchronize(func() {
			if ctx.Err() == nil {
				a.accept(ev)
			}
		})
	})
}

func (a *app) accept(ev sampleEvent) {
	a.samples = append(a.samples, ev)
	a.sessionPings++
	a.pendingSamples = append(a.pendingSamples, historySampleFromEvent(ev))
	cutoff := time.Now().Add(-130 * time.Second)
	first := 0
	for first < len(a.samples) && a.samples[first].at.Before(cutoff) {
		first++
	}
	if first > 0 {
		a.samples = append([]sampleEvent(nil), a.samples[first:]...)
	}
	if ev.aggregate != nil {
		a.aggregates = append(a.aggregates, *ev.aggregate)
		a.pendingAggregates = append(a.pendingAggregates, historyAggregateFromPoint(*ev.aggregate))
		a.trimAggregates()
	}

	a.updateCurrentLabel()
	now := time.Now()
	if flushed, err := a.maybeFlushAutosaveHistory(now); err == nil && flushed {
		a.lastHistorySave = now
	}
	a.invalidateCharts()
}

func (a *app) shouldFlushAutosaveHistory(now time.Time) bool {
	if a.sessionPings < minAutosaveHistorySamples {
		return false
	}
	if len(a.pendingSamples) == 0 && len(a.pendingAggregates) == 0 {
		return false
	}
	if a.autosavePath == "" {
		return true
	}
	if a.lastHistorySave.IsZero() {
		return true
	}
	return !now.Before(a.lastHistorySave.Add(autosaveHistoryInterval))
}

func (a *app) maybeFlushAutosaveHistory(now time.Time) (bool, error) {
	if !a.shouldFlushAutosaveHistory(now) {
		return false, nil
	}
	if err := a.flushAutosaveHistory(); err != nil {
		return false, err
	}
	return true, nil
}

func (a *app) flushAutosaveHistory() error {
	if a.sessionPings < minAutosaveHistorySamples {
		return nil
	}
	if len(a.pendingSamples) == 0 && len(a.pendingAggregates) == 0 {
		return nil
	}
	path, err := a.ensureAutosaveHistoryPath()
	if err != nil {
		return err
	}
	return a.appendPendingHistory(path)
}

func (a *app) ensureAutosaveHistoryPath() (string, error) {
	if a.autosavePath != "" {
		return a.autosavePath, nil
	}
	path, err := createAutosaveHistoryFile(autosaveHistoryDir(), a.startedAt, os.Getpid())
	if err != nil {
		return "", err
	}
	a.autosavePath = path
	return path, nil
}

func (a *app) saveHistoryDialog() {
	dlg := new(walk.FileDialog)
	dlg.Title = "Save Pingaro History"
	dlg.Filter = "Pingaro History (*.json)|*.json|All Files (*.*)|*.*"
	dlg.FilePath = activeDefaultHistoryPath()
	if ok, err := dlg.ShowSave(a.MainWindow); err != nil {
		walk.MsgBox(a.MainWindow, "Pingaro", err.Error(), walk.MsgBoxIconError)
		return
	} else if !ok {
		return
	}
	a.saveInputs()
	if err := a.saveHistory(dlg.FilePath); err != nil {
		walk.MsgBox(a.MainWindow, "Pingaro", err.Error(), walk.MsgBoxIconError)
		return
	}
}

func (a *app) loadHistoryDialog() {
	dlg := new(walk.FileDialog)
	dlg.Title = "Load Pingaro History"
	dlg.Filter = "Pingaro History (*.json)|*.json|All Files (*.*)|*.*"
	dlg.FilePath = activeDefaultHistoryPath()
	if ok, err := dlg.ShowOpen(a.MainWindow); err != nil {
		walk.MsgBox(a.MainWindow, "Pingaro", err.Error(), walk.MsgBoxIconError)
		return
	} else if !ok {
		return
	}
	if err := a.loadHistory(dlg.FilePath); err != nil {
		walk.MsgBox(a.MainWindow, "Pingaro", err.Error(), walk.MsgBoxIconError)
	}
}

func (a *app) saveHistory(path string) error {
	a.saveInputs()
	if path == "" {
		path = activeDefaultHistoryPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if a.autosavePath != "" && samePath(path, a.autosavePath) {
		return a.flushAutosaveHistory()
	}

	records, err := a.historyRecordsForSave()
	if err != nil {
		return err
	}
	return writeHistoryRecords(path, records)
}

func (a *app) historyRecordsForSave() ([]historyFile, error) {
	if a.autosavePath != "" {
		records, err := readHistoryFile(a.autosavePath)
		if err == nil {
			if len(a.pendingSamples) > 0 || len(a.pendingAggregates) > 0 {
				records = append(records, a.pendingHistorySnapshot())
			}
			if len(records) > 0 {
				return records, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return []historyFile{a.historySnapshot(a.samples, a.aggregates)}, nil
}

func (a *app) appendPendingHistory(path string) error {
	a.saveInputs()
	if len(a.pendingSamples) == 0 && len(a.pendingAggregates) == 0 {
		return nil
	}
	if path == "" {
		path = activeDefaultHistoryPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	h := a.pendingHistorySnapshot()
	if err := appendHistoryLine(path, h); err != nil {
		return err
	}
	a.pendingSamples = nil
	a.pendingAggregates = nil
	return nil
}

func (a *app) pendingHistorySnapshot() historyFile {
	return historyFile{
		Version:       1,
		SavedAt:       time.Now(),
		Config:        a.currentConfig(),
		PeriodSeconds: max(1, int(a.period.Seconds())),
		Samples:       append([]historySample(nil), a.pendingSamples...),
		Aggregates:    append([]historyAggregate(nil), a.pendingAggregates...),
	}
}

func (a *app) historySnapshot(samples []sampleEvent, aggregates []aggregatePoint) historyFile {
	h := historyFile{
		Version:       1,
		SavedAt:       time.Now(),
		Config:        a.currentConfig(),
		PeriodSeconds: max(1, int(a.period.Seconds())),
	}
	for _, s := range samples {
		h.Samples = append(h.Samples, historySampleFromEvent(s))
	}
	for _, p := range aggregates {
		h.Aggregates = append(h.Aggregates, historyAggregateFromPoint(p))
	}
	return h
}

func writeHistoryFile(path string, h historyFile) error {
	return history.WriteFile(path, h)
}

func writeHistoryRecords(path string, records []historyFile) error {
	return history.WriteRecords(path, records)
}

func appendHistoryLine(path string, h historyFile) error {
	return history.AppendLine(path, h)
}

func readHistoryFile(path string) ([]historyFile, error) {
	return history.ReadFile(path)
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err == nil {
		a = aa
	}
	bb, err := filepath.Abs(b)
	if err == nil {
		b = bb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func historySampleFromEvent(s sampleEvent) historySample {
	rtt := s.rtt
	if s.lost {
		rtt = historyLostRTT
	}
	return historySample{
		At:          s.at,
		GroupIndex:  s.groupID.Index(),
		GroupName:   s.targetLabel,
		RTT:         rtt,
		Lost:        s.lost,
		MinRTT:      s.minRTT,
		MaxRTT:      s.maxRTT,
		Total:       s.total,
		LostTotal:   s.lostTotal,
		LossPercent: s.lossPercent,
		P95:         s.p95,
		JitterP95:   s.jitterP95,
		WindowLoss:  s.windowLoss,
	}
}

func historyAggregateFromPoint(p aggregatePoint) historyAggregate {
	return historyAggregate{
		At:         p.at,
		GroupIndex: p.groupID.Index(),
		GroupName:  p.groupName,
		P95:        p.p95,
		Loss:       p.loss,
		JitterP95:  p.jitterP95,
	}
}

func (a *app) loadHistory(path string) error {
	records, err := readHistoryFile(path)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("history file is empty")
	}

	a.stop()
	a.samples = nil
	a.aggregates = nil
	a.pendingSamples = nil
	a.pendingAggregates = nil
	a.autosavePath = ""
	a.sessionPings = 0
	a.startedAt = time.Time{}
	a.aggregateEmptyAt = time.Time{}
	a.historyViewEnd = time.Time{}
	for _, h := range records {
		if h.Version != 1 {
			return fmt.Errorf("unsupported history version %d", h.Version)
		}
		if a.aggregateEmptyAt.IsZero() && !h.SavedAt.IsZero() {
			a.aggregateEmptyAt = h.SavedAt
		}
		if len(h.Config.Groups) > 0 {
			a.applyConfig(h.Config)
		}
		a.period = time.Duration(max(1, h.PeriodSeconds)) * time.Second
		for _, s := range h.Samples {
			idx := clampInt(s.GroupIndex, 0, len(a.colors)-1)
			name := s.GroupName
			if name == "" {
				name = fmt.Sprintf("Group %d", idx+1)
			}
			rtt := s.RTT
			if s.Lost {
				rtt = 0
			}
			a.samples = append(a.samples, sampleEvent{
				at:          s.At,
				groupID:     groupIDFromIndex(idx),
				rtt:         rtt,
				replied:     !s.Lost,
				lost:        s.Lost,
				targetLabel: name,
				minRTT:      s.MinRTT,
				maxRTT:      s.MaxRTT,
				total:       s.Total,
				lostTotal:   s.LostTotal,
				lossPercent: s.LossPercent,
				p95:         s.P95,
				jitterP95:   s.JitterP95,
				windowLoss:  s.WindowLoss,
				period:      a.period,
			})
			a.sessionPings++
			if a.startedAt.IsZero() || s.At.Before(a.startedAt) {
				a.startedAt = s.At
			}
			if s.At.After(a.historyViewEnd) {
				a.historyViewEnd = s.At
			}
		}
		for _, p := range h.Aggregates {
			idx := clampInt(p.GroupIndex, 0, len(a.colors)-1)
			name := p.GroupName
			if name == "" {
				name = fmt.Sprintf("Group %d", idx+1)
			}
			a.aggregates = append(a.aggregates, aggregatePoint{
				groupID:   groupIDFromIndex(idx),
				groupName: name,
				at:        p.At,
				p95:       p.P95,
				loss:      p.Loss,
				jitterP95: p.JitterP95,
			})
			if p.At.After(a.historyViewEnd) {
				a.historyViewEnd = p.At
			}
		}
	}
	if !a.startedAt.IsZero() {
		a.aggregateEmptyAt = a.startedAt
	}
	a.trimAggregates()
	a.refreshMetricsFromLoadedHistory()
	a.invalidateCharts()
	return nil
}

func parseHistoryRecords(data []byte) ([]historyFile, error) {
	return history.ParseRecords(data)
}

func (a *app) trimAggregates() {
	keep := a.maxAggregatePoints()
	if len(a.aggregates) <= keep {
		return
	}
	a.aggregates = append([]aggregatePoint(nil), a.aggregates[len(a.aggregates)-keep:]...)
}

func (a *app) maxAggregatePoints() int {
	groupCount := max(1, len(a.currentSavedGroups()))
	return maxAggregatePointsForWidth(groupCount, a.currentScreenWidthPixels())
}

func (a *app) currentScreenWidthPixels() int {
	if a != nil && a.MainWindow != nil {
		hwnd := a.MainWindow.Handle()
		if hwnd != 0 {
			var mi win.MONITORINFO
			mi.CbSize = uint32(unsafe.Sizeof(mi))
			if win.GetMonitorInfo(win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST), &mi) {
				if width := int(mi.RcMonitor.Right - mi.RcMonitor.Left); width > 0 {
					return width
				}
			}
		}
		if width := a.MainWindow.BoundsPixels().Width; width > 0 {
			return width
		}
	}
	if width := int(win.GetSystemMetrics(win.SM_CXSCREEN)); width > 0 {
		return width
	}
	return initialWindowWidth
}

func maxAggregatePointsForWidth(groupCount, screenWidth int) int {
	groupCount = max(1, groupCount)
	return groupCount * aggregatePointCapacity(screenWidth, aggregateMinPixelsPerSample)
}

func (a *app) currentConfig() savedConfig {
	return savedConfig{
		Groups:             a.currentSavedGroups(),
		PPS:                clampInt(parseInt(a.pps.Text(), 1), 1, 20),
		AggregationSeconds: clampInt(parseInt(a.agg.Text(), 120), 3, 3600),
		UseTypes:           a.selectedUseTypes(),
	}
}

func (a *app) currentSavedGroups() []savedGroup {
	out := make([]savedGroup, 0, 3)
	for i := 0; i < 3; i++ {
		targets := strings.Join(parseTargets(a.groupTargets[i].Text()), ", ")
		if targets == "" {
			continue
		}
		name := strings.TrimSpace(a.groupNames[i].Text())
		if name == "" {
			name = fmt.Sprintf("Group %d", i+1)
		}
		out = append(out, savedGroup{Name: name, Targets: targets})
	}
	return out
}

func (a *app) applyConfig(cfg savedConfig) {
	cfg.PPS = clampInt(cfg.PPS, 1, 20)
	cfg.AggregationSeconds = clampInt(cfg.AggregationSeconds, 3, 3600)
	cfg.UseTypes = normalizeUseTypes(cfg.UseTypes, cfg.UseType)
	cfg.UseType = ""
	cfg.Groups = normalizeSavedGroups(cfg.Groups)
	for i := 0; i < 3; i++ {
		name, targets := "", ""
		if i < len(cfg.Groups) {
			name = cfg.Groups[i].Name
			targets = cfg.Groups[i].Targets
		}
		a.groupNames[i].SetText(name)
		a.groupTargets[i].SetText(targets)
	}
	a.pps.SetText(strconv.Itoa(cfg.PPS))
	a.agg.SetText(strconv.Itoa(cfg.AggregationSeconds))
	selected := useTypeSet(cfg.UseTypes)
	for i, profile := range useProfiles {
		if a.useChecks[i] != nil {
			a.useChecks[i].SetChecked(selected[profile.Name])
		}
	}
	a.ensureUseSelection()
	a.settings = cfg
	a.updateJitterChartVisibility()
	a.invalidateCharts()
	saveConfig(cfg)
}

func (a *app) refreshMetricsFromLoadedHistory() {
	a.updateCurrentLabel()
}

func (a *app) updateCurrentLabel() {
	if a.sessionPings == 0 {
		a.currentLabel.SetText("No measurements yet")
		a.setConnectionSummary("", 0, false)
		return
	}
	end := time.Now()
	if !a.historyViewEnd.IsZero() {
		end = a.historyViewEnd
	}
	start := a.startedAt
	if start.IsZero() {
		start = end
	}
	a.currentLabel.SetText(fmt.Sprintf("%d pings, %s", a.sessionPings, formatDuration(end.Sub(start))))
	text, severity := summarizeConnectionIssues(a.connectionIssues())
	a.setConnectionSummary(text, severity, true)
}

func (a *app) setConnectionSummary(text string, severity int, visible bool) {
	if a.summaryPanel == nil || a.summaryLabel == nil {
		return
	}
	_ = a.summaryLabel.SetText(text)
	if severity != a.summarySeverity {
		brush, err := walk.NewSolidColorBrush(summaryBackgroundColor(severity))
		if err == nil {
			a.summaryPanel.SetBackground(brush)
			a.summarySeverity = severity
		}
	}
	a.summaryPanel.SetVisible(visible)
	if parent := a.summaryPanel.Parent(); parent != nil {
		parent.RequestLayout()
	}
}

func (a *app) connectionIssues() []connectionIssue {
	if len(a.aggregates) == 0 {
		return a.realtimeConnectionIssues()
	}
	return a.aggregateConnectionIssues()
}

func (a *app) aggregateConnectionIssues() []connectionIssue {
	profile := a.selectedProfile()
	includeJitter := a.shouldShowJitterChart()
	issues := make([]connectionIssue, 0, len(a.aggregates))
	for _, p := range a.aggregates {
		severity := thresholdSeverity(p.p95, profile.RTT)
		severity = max(severity, thresholdSeverity(p.loss, profile.Loss))
		if includeJitter {
			severity = max(severity, thresholdSeverity(p.jitterP95, profile.Jitter))
		}
		if severity > 0 {
			issues = append(issues, connectionIssue{at: p.at, severity: severity})
		}
	}
	return issues
}

func (a *app) realtimeConnectionIssues() []connectionIssue {
	points, _, _ := a.rttPoints()
	issues := make([]connectionIssue, 0, len(points))
	for _, p := range points {
		if p.severity > 0 {
			issues = append(issues, connectionIssue{at: p.at, severity: p.severity})
		}
	}
	return issues
}

func summarizeConnectionIssues(issues []connectionIssue) (string, int) {
	if len(issues) == 0 {
		return "So far, the connection looks good.", 0
	}

	buckets := map[int]*connectionIssueBucket{}
	maxSeverity := 0
	for _, issue := range issues {
		if issue.severity <= 0 {
			continue
		}
		severity := clampInt(issue.severity, 1, 3)
		bucket := buckets[severity]
		if bucket == nil {
			bucket = &connectionIssueBucket{severity: severity, first: issue.at, last: issue.at}
			buckets[severity] = bucket
		}
		bucket.count++
		if issue.at.Before(bucket.first) {
			bucket.first = issue.at
		}
		if issue.at.After(bucket.last) {
			bucket.last = issue.at
		}
		maxSeverity = max(maxSeverity, severity)
	}
	if len(buckets) == 0 {
		return "So far, the connection looks good.", 0
	}

	severities := make([]int, 0, len(buckets))
	for severity := range buckets {
		severities = append(severities, severity)
	}
	sort.Ints(severities)

	parts := make([]string, 0, len(severities))
	combined := len(severities) > 1
	for _, severity := range severities {
		parts = append(parts, issueBucketPhrase(*buckets[severity], combined))
	}
	if combined {
		return "The connection has had: " + strings.Join(parts, ", ") + ".", maxSeverity
	}
	return "The connection has had " + parts[0] + ".", maxSeverity
}

func issueBucketPhrase(bucket connectionIssueBucket, combined bool) string {
	severity := issueSeverityName(bucket.severity)
	if bucket.count == 1 {
		return fmt.Sprintf("a %s issue at %s", severity, formatIssueTime(bucket.last))
	}

	countWord := "some"
	if bucket.count >= 4 {
		countWord = "several"
	}
	if sameIssueMinute(bucket.first, bucket.last) {
		return fmt.Sprintf("%s %s issues (at %s)", countWord, severity, formatIssueTime(bucket.last))
	}
	if combined && bucket.count < 4 {
		return fmt.Sprintf("%s %s issues (last at %s)", countWord, severity, formatIssueTime(bucket.last))
	}
	return fmt.Sprintf("%s %s issues (between %s-%s)", countWord, severity, formatIssueTime(bucket.first), formatIssueTime(bucket.last))
}

func issueSeverityName(severity int) string {
	switch severity {
	case 1:
		return "minor"
	case 2:
		return "noticeable"
	default:
		return "serious"
	}
}

func sameIssueMinute(a, b time.Time) bool {
	return a.Format("15:04") == b.Format("15:04")
}

func formatIssueTime(t time.Time) string {
	return t.Format("15:04")
}

func summaryBackgroundColor(severity int) walk.Color {
	if severity == 0 {
		return walk.RGB(57, 231, 95)
	}
	return severityColor(severity)
}

func (a *app) invalidateCharts() {
	a.updateAdaptiveChartHeights()
	for _, chart := range []*walk.CustomWidget{a.rttChart, a.p95Chart, a.lossChart, a.jitterChart} {
		if chart != nil && chart.Visible() {
			chart.Invalidate()
		}
	}
}

func (a *app) updateAdaptiveChartHeights() {
	if a.rttChart == nil || a.p95Chart == nil || a.lossChart == nil || a.jitterChart == nil {
		return
	}
	showJitter := a.shouldShowJitterChart()
	a.jitterChart.SetVisible(showJitter)
	jitterHeight := 0
	if showJitter {
		jitterPoints, _ := a.jitterPoints()
		jitterHeight = adaptiveJitterHeight(jitterPoints)
	}
	heights := redistributedChartHeights(adaptiveLossHeight(a.lossPoints()), jitterHeight, showJitter)
	a.setChartHeight(a.rttChart, &a.rttChartHeight, heights[0])
	a.setChartHeight(a.p95Chart, &a.p95ChartHeight, heights[1])
	a.setChartHeight(a.lossChart, &a.lossChartHeight, heights[2])
	a.setChartHeight(a.jitterChart, &a.jitterChartHeight, heights[3])
}

func (a *app) setChartHeight(chart *walk.CustomWidget, current *int, height int) {
	if height < 0 || *current == height {
		return
	}
	*current = height
	if parent := chart.Parent(); parent != nil {
		if layout := parent.Layout(); layout != nil {
			if stretchLayout, ok := layout.(interface {
				SetStretchFactor(walk.Widget, int) error
			}); ok {
				_ = stretchLayout.SetStretchFactor(chart, height)
			}
		}
	}
}

func chartWidgetMinSize() Size {
	return Size{Width: 0, Height: chartMinHeight}
}

func combinedChartHeight(chartHeight int) int {
	return chartHeaderHeight + headerChartGap + chartHeight
}

func redistributedChartHeights(lossHeight, jitterHeight int, includeJitter bool) [4]int {
	base := [4]int{rttChartHeight, aggregateChartHeight, aggregateChartHeight, aggregateChartHeight}
	heights := [4]int{base[0], base[1], clampInt(lossHeight, 1, base[2]), 0}
	if includeJitter {
		heights[3] = clampInt(jitterHeight, 1, base[3])
	}
	saved := 0
	for i := range heights {
		saved += base[i] - heights[i]
	}
	if saved <= 0 {
		return heights
	}

	receiverWeight := 0
	for i := range heights {
		if i == 3 && !includeJitter {
			continue
		}
		if heights[i] == base[i] {
			receiverWeight += base[i]
		}
	}
	if receiverWeight <= 0 {
		return heights
	}

	remaining := saved
	lastReceiver := -1
	for i := range heights {
		if i == 3 && !includeJitter {
			continue
		}
		if heights[i] != base[i] {
			continue
		}
		lastReceiver = i
		add := saved * base[i] / receiverWeight
		heights[i] += add
		remaining -= add
	}
	if lastReceiver >= 0 {
		heights[lastReceiver] += remaining
	}
	return heights
}

func splitChartBounds(rect walk.Rectangle) (header walk.Rectangle, chart walk.Rectangle) {
	headerHeight := min(chartHeaderHeight, rect.Height)
	header = walk.Rectangle{X: rect.X, Y: rect.Y, Width: rect.Width, Height: headerHeight}
	chartY := rect.Y + headerHeight + headerChartGap
	chartHeight := max(1, rect.Y+rect.Height-chartY)
	chart = walk.Rectangle{X: rect.X, Y: chartY, Width: rect.Width, Height: chartHeight}
	return header, chart
}

func (a *app) rttPoints() ([]chartPoint, time.Time, float64) {
	now := time.Now()
	if !a.historyViewEnd.IsZero() {
		now = a.historyViewEnd
	}
	points := make([]chartPoint, 0, len(a.samples))
	lossWindows := map[int][]bool{}
	profile := a.selectedProfile()
	for _, s := range a.samples {
		if s.at.Before(now.Add(-120 * time.Second)) {
			continue
		}
		groupIndex := s.groupID.Index()
		value := 0.0
		hasValue := false
		severity := 0
		lossWindows[groupIndex] = append(lossWindows[groupIndex], s.lost)
		if len(lossWindows[groupIndex]) > 10 {
			lossWindows[groupIndex] = lossWindows[groupIndex][len(lossWindows[groupIndex])-10:]
		}
		if s.lost {
			lostCount := 0
			for _, lost := range lossWindows[groupIndex] {
				if lost {
					lostCount++
				}
			}
			severity = min(3, lostCount)
		} else if s.replied {
			value = float64(s.rtt)
			hasValue = true
			severity = thresholdSeverity(value, profile.RTT)
		}
		points = append(points, chartPoint{at: s.at, value: value, hasValue: hasValue, lost: s.lost, groupIndex: groupIndex, groupName: s.targetLabel, color: a.groupColor(s.groupID), severity: severity})
	}
	maxValue := bucketedYMax(points, rttYMaxBuckets)
	return points, now, maxValue
}

func (a *app) paintRTT(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points, now, _ := a.rttPoints()
	headerRect, chartRect := splitChartBounds(a.rttChart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "Latency (live)", lastItems(points, "ms", true)); err != nil {
		return err
	}
	plot := plotBounds(chartRect)
	samples := visibleRealtimeBarSamples(points, plot.Width)
	maxValue := bucketedYMax(realtimeBarSamplePoints(samples), rttYMaxBuckets)
	start, end := realtimeBarTimeRange(samples, now, plot.Width)
	return drawRealtimeBarChart(canvas, chartRect, samples, start, end, 20*time.Second, 0, maxValue, "ms")
}

func (a *app) p95Points() ([]chartPoint, float64) {
	points := make([]chartPoint, 0, len(a.aggregates))
	profile := a.selectedProfile()
	for _, p := range a.aggregates {
		points = append(points, chartPoint{at: p.at, value: p.p95, hasValue: true, groupIndex: p.groupID.Index(), groupName: p.groupName, color: a.groupColor(p.groupID), severity: thresholdSeverity(p.p95, profile.RTT)})
	}
	maxValue := bucketedYMax(points, rttYMaxBuckets)
	return points, maxValue
}

func (a *app) paintP95(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points, maxValue := a.p95Points()
	headerRect, chartRect := splitChartBounds(a.p95Chart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "Latency per "+periodLabel(a.period)+" (p95 of RTT)", lastItems(points, "ms", false)); err != nil {
		return err
	}
	return a.drawAggregateChart(canvas, chartRect, points, maxValue, "ms", walk.RGB(40, 150, 135))
}

func (a *app) lossPoints() []chartPoint {
	points := make([]chartPoint, 0, len(a.aggregates))
	profile := a.selectedProfile()
	for _, p := range a.aggregates {
		points = append(points, chartPoint{at: p.at, value: p.loss, hasValue: true, groupIndex: p.groupID.Index(), groupName: p.groupName, color: a.groupColor(p.groupID), severity: thresholdSeverity(p.loss, profile.Loss)})
	}
	return points
}

func (a *app) paintLoss(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points := a.lossPoints()
	headerRect, chartRect := splitChartBounds(a.lossChart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "Packet loss per "+periodLabel(a.period)+" (%)", lastItems(points, "%", false)); err != nil {
		return err
	}
	return a.drawAggregateChart(canvas, chartRect, points, bucketedYMax(points, lossYMaxBuckets), "%", walk.RGB(200, 75, 88))
}

func (a *app) jitterPoints() ([]chartPoint, float64) {
	points := make([]chartPoint, 0, len(a.aggregates))
	profile := a.selectedProfile()
	for _, p := range a.aggregates {
		points = append(points, chartPoint{at: p.at, value: p.jitterP95, hasValue: true, groupIndex: p.groupID.Index(), groupName: p.groupName, color: a.groupColor(p.groupID), severity: thresholdSeverity(p.jitterP95, profile.Jitter)})
	}
	maxValue := bucketedYMax(points, jitterYMaxBuckets)
	return points, maxValue
}

func (a *app) paintJitter(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points, maxValue := a.jitterPoints()
	headerRect, chartRect := splitChartBounds(a.jitterChart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "One-way jitter per "+periodLabel(a.period)+" (p95)", lastItems(points, "ms", false)); err != nil {
		return err
	}
	return a.drawAggregateChart(canvas, chartRect, points, maxValue, "ms", walk.RGB(215, 160, 70))
}

func (a *app) drawAggregateChart(canvas *walk.Canvas, rect walk.Rectangle, points []chartPoint, maxValue float64, unit string, color walk.Color) error {
	plot := plotBounds(rect)
	points = visibleAggregatePoints(points, plot.Width)
	start, end := aggregateChartTimeRange(points, plot.Width, a.period, a.aggregateEmptyStart())
	return drawTimeChart(canvas, rect, points, start, end, 10*time.Minute, 0, maxValue, unit, color)
}

func (a *app) aggregateEmptyStart() time.Time {
	if a.aggregateEmptyAt.IsZero() {
		switch {
		case !a.startedAt.IsZero():
			a.aggregateEmptyAt = a.startedAt
		case !a.historyViewEnd.IsZero():
			a.aggregateEmptyAt = a.historyViewEnd
		default:
			a.aggregateEmptyAt = time.Now()
		}
	}
	return a.aggregateEmptyAt
}

func visibleAggregatePoints(points []chartPoint, plotWidth int) []chartPoint {
	visibleLimit := aggregateVisiblePointLimit(points, plotWidth)
	if len(points) <= visibleLimit {
		return points
	}
	return points[len(points)-visibleLimit:]
}

func aggregateVisiblePointLimit(points []chartPoint, plotWidth int) int {
	return chartPointGroupCount(points) * aggregatePointCapacity(plotWidth, aggregateMinPixelsPerSample)
}

func aggregateChartTimeRange(points []chartPoint, plotWidth int, period time.Duration, emptyStart time.Time) (time.Time, time.Time) {
	if period <= 0 {
		period = time.Second
	}
	preferredPoints := aggregatePointCapacity(plotWidth, aggregatePreferredPixelsPerSample)
	if emptyStart.IsZero() {
		emptyStart = time.Unix(0, 0)
	}
	if len(points) == 0 {
		return emptyStart, emptyStart.Add(period * time.Duration(max(1, preferredPoints-1)))
	}

	start := points[0].at
	end := points[len(points)-1].at
	if aggregateMaxPointsPerGroup(points) <= preferredPoints {
		start = end.Add(-period * time.Duration(max(1, preferredPoints-1)))
		if points[0].at.Before(start) {
			start = points[0].at
		}
	}
	if !end.After(start) {
		end = start.Add(period)
	}
	return start, end
}

func aggregatePointCapacity(width, pixelsPerSample int) int {
	if width < 1 {
		width = 1
	}
	if pixelsPerSample < 1 {
		pixelsPerSample = 1
	}
	return max(1, width/pixelsPerSample)
}

func chartPointGroupCount(points []chartPoint) int {
	groups := map[int]struct{}{}
	for _, p := range points {
		groups[p.groupIndex] = struct{}{}
	}
	return max(1, len(groups))
}

func aggregateMaxPointsPerGroup(points []chartPoint) int {
	counts := map[int]int{}
	maxCount := 0
	for _, p := range points {
		counts[p.groupIndex]++
		maxCount = max(maxCount, counts[p.groupIndex])
	}
	return max(1, maxCount)
}

func visibleRealtimeBarSamples(points []chartPoint, plotWidth int) []realtimeBarSample {
	samples := realtimeBarSamples(points)
	visibleLimit := realtimeSampleCapacity(plotWidth)
	if len(samples) <= visibleLimit {
		return samples
	}
	return samples[len(samples)-visibleLimit:]
}

func realtimeBarSamples(points []chartPoint) []realtimeBarSample {
	samples := make([]realtimeBarSample, 0, len(points))
	for _, p := range points {
		if len(samples) == 0 || !p.at.Equal(samples[len(samples)-1].at) {
			samples = append(samples, realtimeBarSample{at: p.at})
		}
		samples[len(samples)-1].points = append(samples[len(samples)-1].points, p)
	}
	return samples
}

func realtimeSampleCapacity(plotWidth int) int {
	return aggregatePointCapacity(plotWidth, realtimePixelsPerSample)
}

func realtimeBarSamplePoints(samples []realtimeBarSample) []chartPoint {
	count := 0
	for _, sample := range samples {
		count += len(sample.points)
	}
	points := make([]chartPoint, 0, count)
	for _, sample := range samples {
		points = append(points, sample.points...)
	}
	return points
}

func realtimeBarTimeRange(samples []realtimeBarSample, fallbackEnd time.Time, plotWidth int) (time.Time, time.Time) {
	if fallbackEnd.IsZero() {
		fallbackEnd = time.Now()
	}
	if len(samples) == 0 {
		return fallbackEnd.Add(-120 * time.Second), fallbackEnd
	}
	interval := realtimeSampleInterval(samples)
	if interval <= 0 {
		interval = time.Second
	}
	sampleCount := len(samples)
	occupiedWidth := sampleCount * realtimePixelsPerSample
	leftOffset := float64(realtimePixelsPerSample) / 2
	if occupiedWidth < plotWidth {
		leftOffset = float64(plotWidth-occupiedWidth) + float64(realtimePixelsPerSample)/2
	}
	width := max(1, plotWidth)
	startOffsetUnits := leftOffset / float64(realtimePixelsPerSample)
	rangeUnits := float64(width) / float64(realtimePixelsPerSample)
	start := samples[0].at.Add(-scaleDuration(interval, startOffsetUnits))
	end := start.Add(scaleDuration(interval, rangeUnits))
	if !end.After(start) {
		end = start.Add(time.Second)
	}
	return start, end
}

func realtimeSampleInterval(samples []realtimeBarSample) time.Duration {
	if len(samples) < 2 {
		return time.Second
	}
	first := samples[0].at
	last := samples[len(samples)-1].at
	if !last.After(first) {
		return time.Second
	}
	return last.Sub(first) / time.Duration(len(samples)-1)
}

func scaleDuration(d time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return 0
	}
	return time.Duration(math.Round(float64(d) * factor))
}

func realtimeBarSampleSeverity(sample realtimeBarSample) int {
	severity := 0
	for _, p := range sample.points {
		severity = max(severity, p.severity)
	}
	return severity
}

func realtimeBarSegments(points []chartPoint, yMin float64) []realtimeBarSegment {
	usable := make([]chartPoint, 0, len(points))
	for _, p := range points {
		if !p.hasValue {
			continue
		}
		usable = append(usable, p)
	}
	sort.SliceStable(usable, func(i, j int) bool {
		if usable[i].value == usable[j].value {
			return usable[i].groupIndex < usable[j].groupIndex
		}
		return usable[i].value < usable[j].value
	})

	segments := make([]realtimeBarSegment, 0, len(usable))
	previous := yMin
	for _, p := range usable {
		if p.value <= previous {
			continue
		}
		segments = append(segments, realtimeBarSegment{color: p.color, fromValue: previous, toValue: p.value})
		previous = p.value
	}
	return segments
}

func drawRealtimeBarChart(canvas *walk.Canvas, rect walk.Rectangle, samples []realtimeBarSample, start, end time.Time, xGrid time.Duration, yMin, yMax float64, unit string) error {
	if err := drawPanel(canvas, rect); err != nil {
		return err
	}
	plot := plotBounds(rect)
	if !end.After(start) {
		end = start.Add(time.Second)
	}
	if yMax <= yMin {
		yMax = yMin + 1
	}
	if err := drawRealtimeBarHighlights(canvas, plot, samples); err != nil {
		return err
	}
	gridPen, err := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(215, 220, 225))
	if err != nil {
		return err
	}
	defer gridPen.Dispose()
	yDivisions := yGridDivisions(rect.Height)
	for i := 0; i <= yDivisions; i++ {
		value := yMin + (yMax-yMin)*float64(i)/float64(yDivisions)
		y := plot.Y + plot.Height - int(float64(plot.Height)*float64(i)/float64(yDivisions))
		if err := canvas.DrawLinePixels(gridPen, walk.Point{X: plot.X, Y: y}, walk.Point{X: plot.X + plot.Width, Y: y}); err != nil {
			return err
		}
		label := formatAxis(value) + " " + unit
		_ = drawText(canvas, label, walk.Rectangle{X: rect.X + 4, Y: y - 9, Width: plot.X - rect.X - 8, Height: 18}, walk.RGB(80, 90, 100), walk.TextRight|walk.TextVCenter|walk.TextSingleLine)
	}
	if xGrid > 0 {
		duration := end.Sub(start)
		for _, tick := range xAxisTicks(start, end, plot.Width) {
			t := tick.at
			x := plot.X + int(t.Sub(start).Seconds()/duration.Seconds()*float64(plot.Width))
			if err := canvas.DrawLinePixels(gridPen, walk.Point{X: x, Y: plot.Y}, walk.Point{X: x, Y: plot.Y + plot.Height}); err != nil {
				return err
			}
			_ = drawText(canvas, tick.label, walk.Rectangle{X: x - xAxisLabelWidth/2, Y: plot.Y + plot.Height + 3, Width: xAxisLabelWidth, Height: 18}, walk.RGB(80, 90, 100), walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		}
	}
	if len(samples) == 0 {
		return drawText(canvas, "No measurements yet", rect, walk.RGB(120, 130, 140), walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}
	return drawRealtimeBars(canvas, plot, samples, yMin, yMax)
}

func drawRealtimeBarHighlights(canvas *walk.Canvas, plot walk.Rectangle, samples []realtimeBarSample) error {
	brushes := map[walk.Color]*walk.SolidColorBrush{}
	defer disposeSolidBrushes(brushes)
	for i, sample := range samples {
		severity := realtimeBarSampleSeverity(sample)
		if severity == 0 {
			continue
		}
		brush, err := cachedSolidBrush(brushes, severityColor(severity))
		if err != nil {
			return err
		}
		slot := walk.Rectangle{X: realtimeBarSlotLeft(plot, i, len(samples)), Y: plot.Y, Width: realtimePixelsPerSample, Height: plot.Height}
		if err := fillClippedRectanglePixels(canvas, plot, slot, brush); err != nil {
			return err
		}
	}
	return nil
}

func drawRealtimeBars(canvas *walk.Canvas, plot walk.Rectangle, samples []realtimeBarSample, yMin, yMax float64) error {
	brushes := map[walk.Color]*walk.SolidColorBrush{}
	defer disposeSolidBrushes(brushes)
	for i, sample := range samples {
		barX := realtimeBarSlotLeft(plot, i, len(samples)) + realtimeBarHighlightPadding
		for _, segment := range realtimeBarSegments(sample.points, yMin) {
			brush, err := cachedSolidBrush(brushes, segment.color)
			if err != nil {
				return err
			}
			yTop := chartValueY(plot, segment.toValue, yMin, yMax)
			yBottom := chartValueY(plot, segment.fromValue, yMin, yMax)
			top := int(math.Round(yTop))
			bottom := int(math.Round(yBottom))
			if bottom <= top {
				bottom = top + 1
			}
			rect := walk.Rectangle{X: barX, Y: top, Width: realtimeBarWidth, Height: bottom - top}
			if err := fillClippedRectanglePixels(canvas, plot, rect, brush); err != nil {
				return err
			}
		}
		for stackIndex, p := range realtimeBarLossMarkers(sample.points) {
			brush, err := cachedSolidBrush(brushes, p.color)
			if err != nil {
				return err
			}
			if err := drawRealtimeLossMarker(canvas, plot, realtimeLossMarkerRect(plot, barX, stackIndex), brush); err != nil {
				return err
			}
		}
	}
	return nil
}

func realtimeBarLossMarkers(points []chartPoint) []chartPoint {
	markers := make([]chartPoint, 0, len(points))
	for _, p := range points {
		if p.lost {
			markers = append(markers, p)
		}
	}
	sort.SliceStable(markers, func(i, j int) bool {
		return markers[i].groupIndex > markers[j].groupIndex
	})
	return markers
}

func realtimeLossMarkerRect(plot walk.Rectangle, barX int, stackIndex int) walk.Rectangle {
	centerX := barX + realtimeBarWidth/2
	return walk.Rectangle{
		X:      centerX - realtimeLossMarkerSize/2,
		Y:      plot.Y + max(0, stackIndex)*realtimeLossMarkerSize,
		Width:  realtimeLossMarkerSize,
		Height: realtimeLossMarkerSize,
	}
}

func drawRealtimeLossMarker(canvas *walk.Canvas, plot walk.Rectangle, rect walk.Rectangle, brush walk.Brush) error {
	for i := 0; i < realtimeLossMarkerSize; i++ {
		y := rect.Y + min(i, realtimeLossMarkerSize-realtimeLossMarkerStrokeWidth)
		mainX := rect.X + min(i, realtimeLossMarkerSize-realtimeLossMarkerStrokeWidth)
		if err := fillRealtimeLossMarkerStroke(canvas, plot, rect, mainX, y, brush); err != nil {
			return err
		}
		antiX := rect.X + max(0, realtimeLossMarkerSize-realtimeLossMarkerStrokeWidth-i)
		if err := fillRealtimeLossMarkerStroke(canvas, plot, rect, antiX, y, brush); err != nil {
			return err
		}
	}
	return nil
}

func fillRealtimeLossMarkerStroke(canvas *walk.Canvas, plot walk.Rectangle, marker walk.Rectangle, x, y int, brush walk.Brush) error {
	stroke := walk.Rectangle{X: x, Y: y, Width: realtimeLossMarkerStrokeWidth, Height: realtimeLossMarkerStrokeWidth}
	clipped, ok := clippedRectanglePixels(stroke, marker)
	if !ok {
		return nil
	}
	return fillClippedRectanglePixels(canvas, plot, clipped, brush)
}

func realtimeBarSlotLeft(plot walk.Rectangle, sampleIndex, sampleCount int) int {
	occupiedWidth := max(1, sampleCount) * realtimePixelsPerSample
	left := plot.X
	if occupiedWidth < plot.Width {
		left += plot.Width - occupiedWidth
	}
	return left + sampleIndex*realtimePixelsPerSample
}

func chartValueY(plot walk.Rectangle, value, yMin, yMax float64) float64 {
	return float64(plot.Y+plot.Height) - (value-yMin)/(yMax-yMin)*float64(plot.Height)
}

func cachedSolidBrush(brushes map[walk.Color]*walk.SolidColorBrush, color walk.Color) (*walk.SolidColorBrush, error) {
	if brush := brushes[color]; brush != nil {
		return brush, nil
	}
	brush, err := walk.NewSolidColorBrush(color)
	if err != nil {
		return nil, err
	}
	brushes[color] = brush
	return brush, nil
}

func disposeSolidBrushes(brushes map[walk.Color]*walk.SolidColorBrush) {
	for _, brush := range brushes {
		brush.Dispose()
	}
}

func fillClippedRectanglePixels(canvas *walk.Canvas, clip walk.Rectangle, rect walk.Rectangle, brush walk.Brush) error {
	clipped, ok := clippedRectanglePixels(rect, clip)
	if !ok {
		return nil
	}
	return canvas.FillRectanglePixels(brush, clipped)
}

func clippedRectanglePixels(a, b walk.Rectangle) (walk.Rectangle, bool) {
	left := max(a.X, b.X)
	top := max(a.Y, b.Y)
	right := min(a.X+a.Width, b.X+b.Width)
	bottom := min(a.Y+a.Height, b.Y+b.Height)
	if right <= left || bottom <= top {
		return walk.Rectangle{}, false
	}
	return walk.Rectangle{X: left, Y: top, Width: right - left, Height: bottom - top}, true
}

func drawTimeChart(canvas *walk.Canvas, rect walk.Rectangle, points []chartPoint, start, end time.Time, xGrid time.Duration, yMin, yMax float64, unit string, color walk.Color) error {
	if err := drawPanel(canvas, rect); err != nil {
		return err
	}
	plot := plotBounds(rect)
	if !end.After(start) {
		end = start.Add(time.Second)
	}
	if err := drawWarningBars(canvas, plot, points, start, end); err != nil {
		return err
	}
	gridPen, err := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(215, 220, 225))
	if err != nil {
		return err
	}
	defer gridPen.Dispose()
	if yMax <= yMin {
		yMax = yMin + 1
	}
	yDivisions := yGridDivisions(rect.Height)
	for i := 0; i <= yDivisions; i++ {
		value := yMin + (yMax-yMin)*float64(i)/float64(yDivisions)
		y := plot.Y + plot.Height - int(float64(plot.Height)*float64(i)/float64(yDivisions))
		if err := canvas.DrawLinePixels(gridPen, walk.Point{X: plot.X, Y: y}, walk.Point{X: plot.X + plot.Width, Y: y}); err != nil {
			return err
		}
		label := formatAxis(value) + " " + unit
		_ = drawText(canvas, label, walk.Rectangle{X: rect.X + 4, Y: y - 9, Width: plot.X - rect.X - 8, Height: 18}, walk.RGB(80, 90, 100), walk.TextRight|walk.TextVCenter|walk.TextSingleLine)
	}
	if xGrid > 0 {
		duration := end.Sub(start)
		for _, tick := range xAxisTicks(start, end, plot.Width) {
			t := tick.at
			x := plot.X + int(t.Sub(start).Seconds()/duration.Seconds()*float64(plot.Width))
			if err := canvas.DrawLinePixels(gridPen, walk.Point{X: x, Y: plot.Y}, walk.Point{X: x, Y: plot.Y + plot.Height}); err != nil {
				return err
			}
			_ = drawText(canvas, tick.label, walk.Rectangle{X: x - xAxisLabelWidth/2, Y: plot.Y + plot.Height + 3, Width: xAxisLabelWidth, Height: 18}, walk.RGB(80, 90, 100), walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		}
	}
	if len(points) == 0 {
		return drawText(canvas, "No measurements yet", rect, walk.RGB(120, 130, 140), walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}

	byGroup := map[int][]chartPlotPoint{}
	colors := map[int]walk.Color{}
	for _, p := range points {
		if p.at.Before(start) || p.at.After(end) {
			continue
		}
		x := float64(plot.X) + p.at.Sub(start).Seconds()/end.Sub(start).Seconds()*float64(plot.Width)
		y := float64(plot.Y+plot.Height) - (p.value-yMin)/(yMax-yMin)*float64(plot.Height)
		byGroup[p.groupIndex] = append(byGroup[p.groupIndex], chartPlotPoint{x: x, y: y})
		colors[p.groupIndex] = p.color
	}
	for groupIndex, linePoints := range byGroup {
		lineColor := chartLineColor(colors, groupIndex, color)
		pen, err := walk.NewCosmeticPen(walk.PenSolid, lineColor)
		if err != nil {
			return err
		}
		if len(linePoints) == 1 {
			p := linePoints[0]
			err = drawClippedLine(canvas, pen, plot, p.x-1, p.y, p.x+1, p.y)
		} else {
			for i := 1; i < len(linePoints); i++ {
				p1 := linePoints[i-1]
				p2 := linePoints[i]
				if err = drawClippedLine(canvas, pen, plot, p1.x, p1.y, p2.x, p2.y); err != nil {
					break
				}
			}
		}
		pen.Dispose()
		if err != nil {
			return err
		}
	}
	return nil
}

func chartLineColor(colors map[int]walk.Color, groupIndex int, fallback walk.Color) walk.Color {
	if lineColor, ok := colors[groupIndex]; ok {
		return lineColor
	}
	return fallback
}

func drawClippedLine(canvas *walk.Canvas, pen walk.Pen, rect walk.Rectangle, x1, y1, x2, y2 float64) error {
	p1, p2, ok := clipLineToRect(rect, x1, y1, x2, y2)
	if !ok {
		return nil
	}
	return canvas.DrawLinePixels(pen, p1, p2)
}

func clipLineToRect(rect walk.Rectangle, x1, y1, x2, y2 float64) (walk.Point, walk.Point, bool) {
	left := float64(rect.X)
	right := float64(rect.X + rect.Width)
	top := float64(rect.Y)
	bottom := float64(rect.Y + rect.Height)
	dx := x2 - x1
	dy := y2 - y1
	t0 := 0.0
	t1 := 1.0

	if !clipLineEdge(-dx, x1-left, &t0, &t1) ||
		!clipLineEdge(dx, right-x1, &t0, &t1) ||
		!clipLineEdge(-dy, y1-top, &t0, &t1) ||
		!clipLineEdge(dy, bottom-y1, &t0, &t1) {
		return walk.Point{}, walk.Point{}, false
	}

	return walk.Point{
			X: int(math.Round(x1 + t0*dx)),
			Y: int(math.Round(y1 + t0*dy)),
		}, walk.Point{
			X: int(math.Round(x1 + t1*dx)),
			Y: int(math.Round(y1 + t1*dy)),
		}, true
}

func clipLineEdge(p, q float64, t0, t1 *float64) bool {
	if p == 0 {
		return q >= 0
	}
	t := q / p
	if p < 0 {
		if t > *t1 {
			return false
		}
		if t > *t0 {
			*t0 = t
		}
		return true
	}
	if t < *t0 {
		return false
	}
	if t < *t1 {
		*t1 = t
	}
	return true
}

func yGridDivisions(chartHeight int) int {
	if chartHeight <= aggregateChartHeight/3+2 {
		return 1
	}
	if chartHeight <= aggregateChartHeight/2+2 {
		return 2
	}
	return 4
}

var xAxisSteps = []time.Duration{
	time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
	30 * time.Second,
	time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	time.Hour,
	2 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
}

func xAxisTicks(start, end time.Time, plotWidth int) []timeAxisTick {
	if !end.After(start) || plotWidth <= 0 {
		return nil
	}
	maxLabels := maxXAxisLabels(plotWidth)
	minLabels := min(3, maxLabels)
	var best []timeAxisTick
	bestCount := 0
	for _, step := range xAxisSteps {
		ticks := xAxisTicksForStep(start, end, step)
		count := len(ticks)
		if count == 0 || count > maxLabels {
			continue
		}
		if count >= minLabels {
			if best == nil || count > bestCount {
				best = ticks
				bestCount = count
			}
			continue
		}
		if best == nil || bestCount < minLabels && count > bestCount {
			best = ticks
			bestCount = count
		}
	}
	if len(best) > 0 {
		return best
	}
	return []timeAxisTick{{at: end, label: xAxisLabel(end)}}
}

func xAxisTicksForStep(start, end time.Time, step time.Duration) []timeAxisTick {
	if step <= 0 {
		return nil
	}
	first := start.Truncate(step)
	if first.Before(start) {
		first = first.Add(step)
	}
	seenLabels := map[string]bool{}
	var ticks []timeAxisTick
	for t := first; !t.After(end); t = t.Add(step) {
		label := xAxisLabel(t)
		if seenLabels[label] {
			continue
		}
		seenLabels[label] = true
		ticks = append(ticks, timeAxisTick{at: t, label: label})
	}
	return ticks
}

func maxXAxisLabels(plotWidth int) int {
	return max(1, int(math.Floor(float64(plotWidth)*0.5/float64(xAxisLabelWidth))))
}

func xAxisLabel(t time.Time) string {
	return t.Local().Format("15:04:05")
}

func drawWarningBars(canvas *walk.Canvas, plot walk.Rectangle, points []chartPoint, start, end time.Time) error {
	if len(points) == 0 || !end.After(start) {
		return nil
	}
	for _, p := range points {
		if p.severity == 0 || p.at.Before(start) || p.at.After(end) {
			continue
		}
		x := plot.X + int(p.at.Sub(start).Seconds()/end.Sub(start).Seconds()*float64(plot.Width))
		brush, err := walk.NewSolidColorBrush(severityColor(p.severity))
		if err != nil {
			return err
		}
		err = canvas.FillRectanglePixels(brush, walk.Rectangle{X: x - 3, Y: plot.Y, Width: 7, Height: plot.Height})
		brush.Dispose()
		if err != nil {
			return err
		}
	}
	return nil
}

func severityColor(severity int) walk.Color {
	switch severity {
	case 1:
		return walk.RGB(193, 193, 200)
	case 2:
		return walk.RGB(252, 234, 144)
	default:
		return walk.RGB(248, 174, 175)
	}
}

func thresholdSeverity(value float64, thresholds [3]float64) int {
	return int(profiles.ThresholdSeverity(value, thresholds))
}

func lastItems(points []chartPoint, unit string, includeGroupName bool) []lastItem {
	if len(points) == 0 {
		return nil
	}
	latest := map[int]chartPoint{}
	for _, p := range points {
		if !p.hasValue && !p.lost {
			continue
		}
		old, ok := latest[p.groupIndex]
		if !ok || p.at.After(old.at) {
			latest[p.groupIndex] = p
		}
	}
	keys := make([]int, 0, len(latest))
	for k := range latest {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	items := make([]lastItem, 0, len(keys))
	for _, k := range keys {
		p := latest[k]
		text := formatNumber(p.value) + " " + unit
		if p.lost {
			text = "lost"
		}
		if includeGroupName {
			text += "(" + p.groupName + ")"
		}
		items = append(items, lastItem{Text: text, Color: p.color})
	}
	return items
}

func drawChartHeader(canvas *walk.Canvas, rect walk.Rectangle, title string, items []lastItem) error {
	titleFont, err := walk.NewFont("Segoe UI", 10, walk.FontBold)
	if err != nil {
		return err
	}
	defer titleFont.Dispose()
	valueFont, err := walk.NewFont("Segoe UI", 10, 0)
	if err != nil {
		return err
	}
	defer valueFont.Dispose()

	total := 0
	var parts []lastItem
	var widths []int
	if len(items) > 0 {
		parts = []lastItem{{Text: "Last = ", Color: walk.RGB(45, 50, 55)}}
		for i, item := range items {
			if i > 0 {
				parts = append(parts, lastItem{Text: " / ", Color: walk.RGB(45, 50, 55)})
			}
			parts = append(parts, item)
		}
		widths = make([]int, len(parts))
	}
	for i, part := range parts {
		w, err := measureTextWidthPixels(canvas, valueFont, part.Text)
		if err != nil {
			return err
		}
		w = max(8, w+2)
		widths[i] = w
		total += w
	}
	titleWidth := rect.Width - total - 12
	if titleWidth < 40 {
		titleWidth = 40
	}
	textHeight := rect.Height
	textY := rect.Y
	if err := canvas.DrawTextPixels(title, titleFont, walk.RGB(30, 35, 40), walk.Rectangle{X: rect.X, Y: textY, Width: titleWidth, Height: textHeight}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	x := rect.X + rect.Width - total
	if x < rect.X {
		x = rect.X
	}
	for i, part := range parts {
		bounds := walk.Rectangle{X: x, Y: textY, Width: widths[i], Height: textHeight}
		if err := canvas.DrawTextPixels(part.Text, valueFont, part.Color, bounds, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine); err != nil {
			return err
		}
		x += widths[i]
	}
	return nil
}

func measureTextWidthPixels(canvas *walk.Canvas, font *walk.Font, text string) (int, error) {
	bounds, _, err := canvas.MeasureTextPixels(text, font, walk.Rectangle{Width: 10000, Height: 1000}, walk.TextSingleLine|walk.TextCalcRect)
	if err != nil {
		return 0, err
	}
	return bounds.Width, nil
}

func drawPanel(canvas *walk.Canvas, rect walk.Rectangle) error {
	bg, err := walk.NewSolidColorBrush(walk.RGB(255, 255, 255))
	if err != nil {
		return err
	}
	defer bg.Dispose()
	if err := canvas.FillRectanglePixels(bg, rect); err != nil {
		return err
	}
	border, err := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(215, 220, 225))
	if err != nil {
		return err
	}
	defer border.Dispose()
	if err := canvas.DrawRectanglePixels(border, walk.Rectangle{X: rect.X, Y: rect.Y, Width: rect.Width - 1, Height: rect.Height - 1}); err != nil {
		return err
	}
	return nil
}

func drawText(canvas *walk.Canvas, text string, rect walk.Rectangle, color walk.Color, format walk.DrawTextFormat) error {
	font, err := walk.NewFont("Segoe UI", 10, 0)
	if err != nil {
		return err
	}
	defer font.Dispose()
	return canvas.DrawTextPixels(text, font, color, rect, format)
}

func inset(r walk.Rectangle, left, top, right, bottom int) walk.Rectangle {
	return walk.Rectangle{X: r.X + left, Y: r.Y + top, Width: max(1, r.Width-left-right), Height: max(1, r.Height-top-bottom)}
}

func plotBounds(r walk.Rectangle) walk.Rectangle {
	return inset(r, 66, 10, 14, 24)
}

func (s *streamState) accept(r pingResult) sampleEvent {
	if s.lastAgg.IsZero() {
		s.lastAgg = r.sentAt
	}
	replied := r.kind == probe.OutcomeReply
	lost := r.kind.CountsAsNetworkLoss()
	if lost {
		s.lostTotal++
	}
	if replied {
		if r.rtt < s.minRTT {
			s.minRTT = r.rtt
		}
		if r.rtt > s.maxRTT {
			s.maxRTT = r.rtt
		}
	}
	s.total++
	s.observations = append(s.observations, observation{rtt: r.rtt, replied: replied, lost: lost})
	keep := max(600, s.aggSeconds*s.pingsPerBatch*2)
	if len(s.observations) > keep {
		s.observations = append([]observation(nil), s.observations[len(s.observations)-keep:]...)
	}
	windowSize := min(len(s.observations), max(1, s.aggSeconds*s.pingsPerBatch))
	window := s.observations[len(s.observations)-windowSize:]
	ok := repliedRTTs(window)
	p95 := 0.0
	if len(ok) > 0 {
		p95 = statsInts(ok).p95
	}
	lostWindow := 0
	for _, obs := range window {
		if obs.lost {
			lostWindow++
		}
	}
	minRTT := s.minRTT
	if minRTT == math.MaxInt {
		minRTT = 0
	}
	ev := sampleEvent{
		at:          r.sentAt,
		groupID:     s.groupID,
		rtt:         r.rtt,
		replied:     replied,
		lost:        lost,
		targetLabel: s.targetLabel,
		minRTT:      minRTT,
		maxRTT:      s.maxRTT,
		total:       s.total,
		lostTotal:   s.lostTotal,
		lossPercent: float64(s.lostTotal) * 100 / float64(s.total),
		p95:         p95,
		jitterP95:   p95Jitter(window),
		windowLoss:  float64(lostWindow) * 100 / float64(len(window)),
		warning:     r.warning,
		period:      time.Duration(s.aggSeconds) * time.Second,
	}
	if r.sentAt.Sub(s.lastAgg) >= time.Duration(s.aggSeconds)*time.Second {
		ev.aggregate = &aggregatePoint{
			groupID:   s.groupID,
			groupName: s.targetLabel,
			at:        r.sentAt,
			p95:       p95,
			loss:      ev.windowLoss,
			jitterP95: ev.jitterP95,
		}
		s.lastAgg = r.sentAt
	}
	return ev
}

func pingBatch(ctx context.Context, hosts []string, destination string, sentAt time.Time) pingResult {
	return pingBatchWithProber(ctx, hosts, destination, sentAt, probe.NewICMPProber())
}

func pingBatchWithProber(ctx context.Context, hosts []string, destination string, sentAt time.Time, prober probe.Prober) pingResult {
	var wg sync.WaitGroup
	ch := make(chan pingResult, len(hosts))
	for i, h := range hosts {
		req := probe.Request{
			ID:       uint64(i + 1),
			Target:   h,
			SentAt:   sentAt,
			Deadline: sentAt.Add(2500 * time.Millisecond),
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- pingResultFromOutcome(prober.Probe(ctx, req))
		}()
	}
	wg.Wait()
	close(ch)
	best := pingResult{sentAt: sentAt, destination: destination, kind: probe.OutcomeLocalFailure}
	warnings := make([]string, 0, len(hosts))
	for r := range ch {
		if r.kind == probe.OutcomeReply {
			if best.kind != probe.OutcomeReply || r.rtt < best.rtt {
				best = r
				best.destination = destination
			}
		} else if r.warning != "" {
			warnings = append(warnings, r.destination+": "+r.warning)
		}
	}
	if best.kind == probe.OutcomeReply {
		return best
	}
	if len(warnings) > 0 {
		best.warning = strings.Join(warnings, " | ")
	}
	return best
}

func pingResultFromOutcome(outcome probe.Outcome) pingResult {
	rtt := 0
	if duration, ok := outcome.RTT(); ok {
		rtt = max(1, int(math.Round(float64(duration)/float64(time.Millisecond))))
	}
	if outcome.Kind() != probe.OutcomeReply || rtt <= 0 {
		rtt = 0
	}
	destination := outcome.Request().Target
	if address, ok := outcome.Address(); ok {
		destination = address.String()
	}
	return pingResult{
		sentAt:      outcome.Request().SentAt,
		rtt:         rtt,
		destination: destination,
		kind:        outcome.Kind(),
		warning:     outcome.Detail(),
	}
}

func pingResultFromBatchResult(result monitor.BatchResult) pingResult {
	rtt := 0
	if result.Kind == probe.OutcomeReply {
		rtt = max(1, int(math.Round(float64(result.RTT)/float64(time.Millisecond))))
	}
	return pingResult{
		sentAt:      result.SentAt,
		rtt:         rtt,
		destination: result.GroupName,
		kind:        result.Kind,
		warning:     result.Warning,
	}
}

type stats struct {
	min, p5, median, p95, max float64
}

func statsInts(values []int) stats {
	if len(values) == 0 {
		return stats{}
	}
	f := make([]float64, 0, len(values))
	for _, v := range values {
		f = append(f, float64(v))
	}
	sort.Float64s(f)
	p5 := max(0, int(float64(len(f))*0.05)-1)
	p95 := max(0, int(float64(len(f))*0.95)-1)
	return stats{min: f[0], p5: f[p5], median: f[len(f)/2], p95: f[p95], max: f[len(f)-1]}
}

func p95Jitter(values []observation) float64 {
	if len(values) < 2 {
		return 0
	}
	var jitters []int
	prev := values[0]
	for _, v := range values[1:] {
		if v.replied && prev.replied {
			jitters = append(jitters, int(math.Round(math.Abs(float64(v.rtt-prev.rtt))/2)))
		}
		prev = v
	}
	if len(jitters) == 0 {
		return 0
	}
	return statsInts(jitters).p95
}

func parseTargets(value string) []string {
	return targets.Parse(value)
}

func resolveTargets(values []string, gateway string) []string {
	return targets.Resolve(values, gateway)
}

func targetListNeedsGateway(values []string) bool {
	return targets.NeedsGateway(values)
}

func repliedRTTs(values []observation) []int {
	out := make([]int, 0, len(values))
	for _, obs := range values {
		if obs.replied {
			out = append(out, obs.rtt)
		}
	}
	return out
}

func parseInt(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return n
}

func clampInt(value, minValue, maxValue int) int {
	return max(minValue, min(maxValue, value))
}

func formatNumber(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return "0"
	}
	if n >= 100 {
		return fmt.Sprintf("%.0f", n)
	}
	if n >= 10 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", n), "0"), ".")
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".")
}

func formatAxis(n float64) string {
	if math.Abs(n) >= 100 {
		return fmt.Sprintf("%.0f", n)
	}
	if math.Abs(n) >= 10 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", n), "0"), ".")
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".")
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(math.Round(d.Seconds()))
	if seconds < 90 {
		unit := "secs"
		if seconds == 1 {
			unit = "sec"
		}
		return fmt.Sprintf("%d %s", seconds, unit)
	}
	minutes := int(math.Round(d.Minutes()))
	if minutes < 90 {
		unit := "mins"
		if minutes == 1 {
			unit = "min"
		}
		return fmt.Sprintf("%d %s", minutes, unit)
	}
	hours := int(math.Round(d.Hours()))
	unit := "hours"
	if hours == 1 {
		unit = "hour"
	}
	return fmt.Sprintf("%d %s", hours, unit)
}

func periodLabel(d time.Duration) string {
	if d <= 0 {
		d = 120 * time.Second
	}
	if d < time.Minute {
		return fmt.Sprintf("%d sec", int(math.Round(d.Seconds())))
	}
	minutes := d.Minutes()
	if math.Abs(minutes-math.Round(minutes)) < 0.05 {
		return fmt.Sprintf("%.0f min", minutes)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", minutes), "0"), ".") + " min"
}

func maxChartValue(points []chartPoint) float64 {
	out := 0.0
	for _, p := range points {
		if p.hasValue && p.value > out {
			out = p.value
		}
	}
	return out
}

func bucketedYMax(points []chartPoint, buckets []float64) float64 {
	if len(buckets) == 0 {
		return 1
	}
	value := percentileValue(points, 0.90)
	if value <= 0 {
		return buckets[0]
	}
	for _, bucket := range buckets {
		if value <= bucket {
			return bucket
		}
	}
	return buckets[len(buckets)-1]
}

func percentileValue(points []chartPoint, percentile float64) float64 {
	values := make([]float64, 0, len(points))
	for _, p := range points {
		if !p.hasValue {
			continue
		}
		if math.IsNaN(p.value) || math.IsInf(p.value, 0) {
			continue
		}
		values = append(values, p.value)
	}
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	index := int(math.Ceil(percentile*float64(len(values)))) - 1
	index = clampInt(index, 0, len(values)-1)
	return values[index]
}

func adaptiveLossHeight(points []chartPoint) int {
	if len(points) == 0 {
		return aggregateChartHeight
	}
	maxLoss := maxChartValue(points)
	if maxLoss < 1 {
		return max(1, aggregateChartHeight/3)
	}
	if maxLoss < 2 {
		return max(1, aggregateChartHeight/2)
	}
	return aggregateChartHeight
}

func adaptiveJitterHeight(points []chartPoint) int {
	if len(points) == 0 {
		return aggregateChartHeight
	}
	maxJitter := maxChartValue(points)
	if maxJitter < 20 {
		return max(1, aggregateChartHeight/3)
	}
	if maxJitter < 30 {
		return max(1, aggregateChartHeight/2)
	}
	return aggregateChartHeight
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
