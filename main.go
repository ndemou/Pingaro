package main

//go:generate go run github.com/akavel/rsrc@v0.10.2 -manifest pingaro.exe.manifest -ico assets/pingaro.ico -o rsrc.syso

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const lostRTT = 9999

const (
	appIconResourceID      = 2
	defaultInternetTargets = "1.1.1.1, 1.1.1.2, 8.8.8.8, 8.8.4.4"
)

const (
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

var (
	rttYMaxBuckets    = []float64{4, 8, 16, 32, 64, 128, 256, 512}
	lossYMaxBuckets   = []float64{2, 4, 8, 16, 32}
	jitterYMaxBuckets = []float64{8, 16, 32, 64, 128, 256}
)

var (
	reTime   = regexp.MustCompile(`time[=<]([0-9]+)ms`)
	reTarget = regexp.MustCompile(`Reply from ([^:]+):`)
)

type pingResult struct {
	sentAt      time.Time
	rtt         int
	destination string
	status      string
	warning     string
}

type sampleEvent struct {
	at          time.Time
	groupIndex  int
	color       walk.Color
	rtt         int
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

type historyFile struct {
	Version       int                `json:"version"`
	SavedAt       time.Time          `json:"savedAt"`
	Config        savedConfig        `json:"config"`
	PeriodSeconds int                `json:"periodSeconds"`
	Samples       []historySample    `json:"samples"`
	Aggregates    []historyAggregate `json:"aggregates"`
}

type historySample struct {
	At          time.Time `json:"at"`
	GroupIndex  int       `json:"groupIndex"`
	GroupName   string    `json:"groupName"`
	RTT         int       `json:"rtt"`
	Lost        bool      `json:"lost"`
	MinRTT      int       `json:"minRtt"`
	MaxRTT      int       `json:"maxRtt"`
	Total       int       `json:"total"`
	LostTotal   int       `json:"lostTotal"`
	LossPercent float64   `json:"lossPercent"`
	P95         float64   `json:"p95"`
	JitterP95   float64   `json:"jitterP95"`
	WindowLoss  float64   `json:"windowLoss"`
}

type historyAggregate struct {
	At         time.Time `json:"at"`
	GroupIndex int       `json:"groupIndex"`
	GroupName  string    `json:"groupName"`
	P95        float64   `json:"p95"`
	Loss       float64   `json:"loss"`
	JitterP95  float64   `json:"jitterP95"`
}

type streamState struct {
	groupIndex    int
	color         walk.Color
	targetLabel   string
	aggSeconds    int
	pingsPerBatch int
	rtts          []int
	total         int
	lostTotal     int
	minRTT        int
	maxRTT        int
	lastAgg       time.Time
}

type aggregatePoint struct {
	groupIndex int
	groupName  string
	color      walk.Color
	at         time.Time
	p95        float64
	loss       float64
	jitterP95  float64
}

type chartPoint struct {
	at         time.Time
	value      float64
	groupIndex int
	groupName  string
	color      walk.Color
	severity   int
}

type chartPlotPoint struct {
	x float64
	y float64
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
	Name    string
	Targets []string
	Color   walk.Color
}

type savedConfig struct {
	Groups             []savedGroup `json:"groups"`
	PPS                int          `json:"pps"`
	AggregationSeconds int          `json:"aggregationSeconds"`
	UseType            string       `json:"useType,omitempty"`
	UseTypes           []string     `json:"useTypes,omitempty"`
}

type savedGroup struct {
	Name    string `json:"name"`
	Targets string `json:"targets"`
}

type useProfile struct {
	Name            string
	RTT             [3]float64
	Loss            [3]float64
	Jitter          [3]float64
	DefaultSelected bool
	ShowsJitter     bool
}

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
	sessionPings      int
	rttChartHeight    int
	p95ChartHeight    int
	lossChartHeight   int
	jitterChartHeight int
	summarySeverity   int
}

var useProfiles = []useProfile{
	{Name: "Browsing & Email", RTT: [3]float64{250, 500, 1000}, Loss: [3]float64{3, 8, 15}, Jitter: [3]float64{80, 150, 300}, DefaultSelected: true},
	{Name: "Remote Desktop", RTT: [3]float64{120, 200, 350}, Loss: [3]float64{1, 3, 8}, Jitter: [3]float64{30, 60, 120}},
	{Name: "Audio Calls", RTT: [3]float64{120, 180, 300}, Loss: [3]float64{1, 3, 6}, Jitter: [3]float64{20, 40, 80}, DefaultSelected: true, ShowsJitter: true},
	{Name: "Video Calls", RTT: [3]float64{150, 250, 400}, Loss: [3]float64{2, 5, 10}, Jitter: [3]float64{30, 60, 120}, DefaultSelected: true, ShowsJitter: true},
	{Name: "Online Gaming", RTT: [3]float64{80, 140, 220}, Loss: [3]float64{0.5, 1.5, 4}, Jitter: [3]float64{15, 30, 60}, DefaultSelected: true},
	{Name: "Superhuman Gaming", RTT: [3]float64{40, 80, 140}, Loss: [3]float64{0.2, 1, 2.5}, Jitter: [3]float64{8, 18, 35}},
}

var useProfileAliases = map[string]string{
	"email & browsing":   "Browsing & Email",
	"low latency gaming": "Superhuman Gaming",
}

func useTypes() []string {
	names := make([]string, 0, len(useProfiles))
	for _, profile := range useProfiles {
		names = append(names, profile.Name)
	}
	return names
}

func defaultUseTypes() []string {
	names := make([]string, 0, len(useProfiles))
	for _, profile := range useProfiles {
		if profile.DefaultSelected {
			names = append(names, profile.Name)
		}
	}
	return names
}

func normalizeUseType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if name, ok := useProfileAliases[value]; ok {
		return name
	}
	for _, profile := range useProfiles {
		if value == strings.ToLower(profile.Name) {
			return profile.Name
		}
	}
	return ""
}

func normalizeUseTypes(values []string, legacyValue string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, value := range values {
		name := normalizeUseType(value)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		normalized = append(normalized, name)
	}
	if len(normalized) == 0 && strings.TrimSpace(legacyValue) != "" {
		if name := normalizeUseType(legacyValue); name != "" {
			normalized = append(normalized, name)
		}
	}
	if len(normalized) == 0 {
		return defaultUseTypes()
	}
	return normalized
}

func useTypeSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func profileFor(name string) (useProfile, bool) {
	name = normalizeUseType(name)
	for _, profile := range useProfiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return useProfile{}, false
}

func profileForUses(names []string) useProfile {
	names = normalizeUseTypes(names, "")
	first, _ := profileFor(names[0])
	combined := first
	combined.Name = strings.Join(names, ", ")
	for _, name := range names[1:] {
		profile, ok := profileFor(name)
		if !ok {
			continue
		}
		for i := 0; i < 3; i++ {
			combined.RTT[i] = math.Min(combined.RTT[i], profile.RTT[i])
			combined.Loss[i] = math.Min(combined.Loss[i], profile.Loss[i])
			combined.Jitter[i] = math.Min(combined.Jitter[i], profile.Jitter[i])
		}
		combined.ShowsJitter = combined.ShowsJitter || profile.ShowsJitter
	}
	return combined
}

func usesShowJitter(names []string) bool {
	for _, name := range normalizeUseTypes(names, "") {
		if profile, ok := profileFor(name); ok && profile.ShowsJitter {
			return true
		}
	}
	return false
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
		walk.RGB(0, 0, 0),
		walk.RGB(133, 0, 135),
		walk.RGB(40, 124, 39),
	}
}

func loadConfig() savedConfig {
	return loadConfigFromPaths(configPath(), legacyConfigPath())
}

func loadConfigFromPaths(path, legacyPath string) savedConfig {
	if cfg, ok := readSavedConfig(path); ok && len(cfg.Groups) > 0 {
		return cfg
	}
	if cfg, ok := readSavedConfig(legacyPath); ok {
		if err := writeConfigFile(path, cfg); err == nil {
			_ = os.Remove(legacyPath)
		}
		if len(cfg.Groups) > 0 {
			return cfg
		}
	}
	return defaultConfig()
}

func readSavedConfig(path string) (savedConfig, bool) {
	cfg := baseConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, false
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	return normalizeLoadedConfig(cfg), true
}

func normalizeLoadedConfig(cfg savedConfig) savedConfig {
	cfg.PPS = clampInt(cfg.PPS, 1, 20)
	cfg.AggregationSeconds = clampInt(cfg.AggregationSeconds, 3, 3600)
	cfg.UseTypes = normalizeUseTypes(cfg.UseTypes, cfg.UseType)
	cfg.UseType = ""
	cfg.Groups = normalizeSavedGroups(cfg.Groups)
	return cfg
}

func baseConfig() savedConfig {
	return savedConfig{PPS: 1, AggregationSeconds: 120, UseTypes: defaultUseTypes()}
}

func defaultConfig() savedConfig {
	cfg := savedConfig{
		PPS:                1,
		AggregationSeconds: 120,
		UseTypes:           defaultUseTypes(),
		Groups:             defaultGroups(),
	}
	return cfg
}

func defaultGroups() []savedGroup {
	return []savedGroup{
		{Name: "Gateway", Targets: "gateway"},
		{Name: "Internet", Targets: defaultInternetTargets},
	}
}

func normalizeSavedGroups(groups []savedGroup) []savedGroup {
	out := make([]savedGroup, 0, 3)
	for _, g := range groups {
		targets := strings.Join(parseTargets(g.Targets), ", ")
		if targets == "" {
			continue
		}
		name := strings.TrimSpace(g.Name)
		if name == "" {
			name = fmt.Sprintf("Group %d", len(out)+1)
		}
		out = append(out, savedGroup{Name: name, Targets: targets})
		if len(out) == 3 {
			break
		}
	}
	return out
}

func saveConfig(cfg savedConfig) {
	_ = writeConfigFile(configPath(), cfg)
}

func writeConfigFile(path string, cfg savedConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
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
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.Rename(legacyPath, path); err == nil {
		return nil
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return os.Remove(legacyPath)
}

func createAutosaveHistoryFile(dir string, startedAt time.Time, pid int) (string, error) {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	if pid <= 0 {
		pid = os.Getpid()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	stem := "history-" + startedAt.Format("2006-01-02_15.04.05")
	for suffix := 0; ; suffix++ {
		path := filepath.Join(dir, autosaveHistoryFilename(stem, pid, suffix))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			closeErr := file.Close()
			if closeErr != nil {
				return "", closeErr
			}
			return path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
}

func autosaveHistoryFilename(stem string, pid, suffix int) string {
	switch suffix {
	case 0:
		return stem + ".json"
	case 1:
		return fmt.Sprintf("%s-pid%d.json", stem, pid)
	default:
		return fmt.Sprintf("%s-pid%d-%d.json", stem, pid, suffix)
	}
}

func appDataPath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "Pingaro", name)
}

func defaultGateway() string {
	if runtime.GOOS == "windows" {
		return defaultGatewayWindows()
	}
	return defaultGatewayUnix()
}

func defaultGatewayWindows() string {
	cmd := exec.Command("route", "PRINT", "-4", "0.0.0.0")
	cmd.SysProcAttr = hiddenSysProcAttr()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	bestGateway := ""
	bestMetric := math.MaxInt
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "0.0.0.0" || fields[1] != "0.0.0.0" {
			continue
		}
		gateway := fields[2]
		if net.ParseIP(gateway) == nil {
			continue
		}
		metric := parseInt(fields[len(fields)-1], math.MaxInt)
		if metric < bestMetric {
			bestMetric = metric
			bestGateway = gateway
		}
	}
	return bestGateway
}

func defaultGatewayUnix() string {
	cmd := exec.Command("sh", "-c", "ip route show default 2>/dev/null | head -n 1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "via" && net.ParseIP(fields[i+1]) != nil {
			return fields[i+1]
		}
	}
	return ""
}

func (a *app) run() error {
	mw := MainWindow{
		AssignTo: &a.MainWindow,
		Title:    "Pingaro - Long term network quality monitor",
		Icon:     appIconResourceID,
		MinSize:  Size{980, 650},
		Size:     Size{1180, 760},
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
	a.sessionPings = 0
	a.lastHistorySave = time.Time{}
	a.setRunning(true)
	a.updateCurrentLabel()
	a.invalidateCharts()
	go a.pingLoop(ctx, groups, pps, aggSeconds)
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
		groups = append(groups, targetGroup{Name: name, Targets: targets, Color: a.colors[i]})
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

func (a *app) pingLoop(ctx context.Context, groups []targetGroup, pps, aggSeconds int) {
	period := time.Second / time.Duration(pps)
	if period < 50*time.Millisecond {
		period = 50 * time.Millisecond
	}
	states := make([]streamState, len(groups))
	for i, g := range groups {
		states[i] = streamState{
			groupIndex:    i,
			color:         g.Color,
			minRTT:        math.MaxInt,
			targetLabel:   g.Name,
			aggSeconds:    aggSeconds,
			pingsPerBatch: pps,
		}
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			events := make([]sampleEvent, len(groups))
			var wg sync.WaitGroup
			for i, g := range groups {
				i, g := i, g
				wg.Add(1)
				go func() {
					defer wg.Done()
					result := pingBatch(ctx, g.Targets, g.Name, t)
					events[i] = states[i].accept(result)
				}()
			}
			wg.Wait()
			a.Synchronize(func() {
				for _, ev := range events {
					a.accept(ev)
				}
			})
		}
	}
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
	return writeHistoryRecords(path, []historyFile{h})
}

func writeHistoryRecords(path string, records []historyFile) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	for _, h := range records {
		if err = encoder.Encode(h); err != nil {
			break
		}
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func appendHistoryLine(path string, h historyFile) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	err = encoder.Encode(h)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readHistoryFile(path string) ([]historyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseHistoryRecords(data)
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
	return historySample{
		At:          s.at,
		GroupIndex:  s.groupIndex,
		GroupName:   s.targetLabel,
		RTT:         s.rtt,
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
		GroupIndex: p.groupIndex,
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
	a.historyViewEnd = time.Time{}
	for _, h := range records {
		if h.Version != 1 {
			return fmt.Errorf("unsupported history version %d", h.Version)
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
			a.samples = append(a.samples, sampleEvent{
				at:          s.At,
				groupIndex:  idx,
				color:       a.colors[idx],
				rtt:         s.RTT,
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
				groupIndex: idx,
				groupName:  name,
				color:      a.colors[idx],
				at:         p.At,
				p95:        p.P95,
				loss:       p.Loss,
				jitterP95:  p.JitterP95,
			})
			if p.At.After(a.historyViewEnd) {
				a.historyViewEnd = p.At
			}
		}
	}
	a.refreshMetricsFromLoadedHistory()
	a.invalidateCharts()
	return nil
}

func parseHistoryRecords(data []byte) ([]historyFile, error) {
	var records []historyFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var h historyFile
		err := decoder.Decode(&h)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid history JSON: %w", err)
		}
		records = append(records, h)
	}
	return records, nil
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
	width := 0
	for _, chart := range []*walk.CustomWidget{a.p95Chart, a.lossChart, a.jitterChart} {
		if chart == nil {
			continue
		}
		if chart == a.jitterChart && !a.shouldShowJitterChart() {
			continue
		}
		width = max(width, plotBounds(chart.ClientBoundsPixels()).Width)
	}
	if width <= 0 {
		width = 1000
	}
	return max(groupCount*width+groupCount*8, groupCount*120)
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
		value := float64(s.rtt)
		severity := 0
		lossWindows[s.groupIndex] = append(lossWindows[s.groupIndex], s.lost)
		if len(lossWindows[s.groupIndex]) > 10 {
			lossWindows[s.groupIndex] = lossWindows[s.groupIndex][len(lossWindows[s.groupIndex])-10:]
		}
		if s.lost {
			value = lostRTT
			lostCount := 0
			for _, lost := range lossWindows[s.groupIndex] {
				if lost {
					lostCount++
				}
			}
			severity = min(3, lostCount)
		} else {
			severity = thresholdSeverity(value, profile.RTT)
		}
		points = append(points, chartPoint{at: s.at, value: value, groupIndex: s.groupIndex, groupName: s.targetLabel, color: s.color, severity: severity})
	}
	maxValue := bucketedYMax(points, rttYMaxBuckets, lostRTT)
	return points, now, maxValue
}

func (a *app) paintRTT(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points, now, maxValue := a.rttPoints()
	headerRect, chartRect := splitChartBounds(a.rttChart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "Latency (live)", lastItems(points, "ms", lostRTT, true)); err != nil {
		return err
	}
	return drawTimeChart(canvas, chartRect, points, now.Add(-120*time.Second), now, 20*time.Second, 0, maxValue, "ms", walk.RGB(40, 150, 135))
}

func (a *app) p95Points() ([]chartPoint, float64) {
	points := make([]chartPoint, 0, len(a.aggregates))
	profile := a.selectedProfile()
	for _, p := range a.aggregates {
		points = append(points, chartPoint{at: p.at, value: p.p95, groupIndex: p.groupIndex, groupName: p.groupName, color: p.color, severity: thresholdSeverity(p.p95, profile.RTT)})
	}
	maxValue := bucketedYMax(points, rttYMaxBuckets, -1)
	return points, maxValue
}

func (a *app) paintP95(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points, maxValue := a.p95Points()
	headerRect, chartRect := splitChartBounds(a.p95Chart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "Latency per "+periodLabel(a.period)+" (p95 of RTT)", lastItems(points, "ms", -1, false)); err != nil {
		return err
	}
	return a.drawAggregateChart(canvas, chartRect, points, maxValue, "ms", walk.RGB(40, 150, 135))
}

func (a *app) lossPoints() []chartPoint {
	points := make([]chartPoint, 0, len(a.aggregates))
	profile := a.selectedProfile()
	for _, p := range a.aggregates {
		points = append(points, chartPoint{at: p.at, value: p.loss, groupIndex: p.groupIndex, groupName: p.groupName, color: p.color, severity: thresholdSeverity(p.loss, profile.Loss)})
	}
	return points
}

func (a *app) paintLoss(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points := a.lossPoints()
	headerRect, chartRect := splitChartBounds(a.lossChart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "Packet loss per "+periodLabel(a.period)+" (%)", lastItems(points, "%", -1, false)); err != nil {
		return err
	}
	return a.drawAggregateChart(canvas, chartRect, points, bucketedYMax(points, lossYMaxBuckets, -1), "%", walk.RGB(200, 75, 88))
}

func (a *app) jitterPoints() ([]chartPoint, float64) {
	points := make([]chartPoint, 0, len(a.aggregates))
	profile := a.selectedProfile()
	for _, p := range a.aggregates {
		points = append(points, chartPoint{at: p.at, value: p.jitterP95, groupIndex: p.groupIndex, groupName: p.groupName, color: p.color, severity: thresholdSeverity(p.jitterP95, profile.Jitter)})
	}
	maxValue := bucketedYMax(points, jitterYMaxBuckets, -1)
	return points, maxValue
}

func (a *app) paintJitter(canvas *walk.Canvas, bounds walk.Rectangle) error {
	points, maxValue := a.jitterPoints()
	headerRect, chartRect := splitChartBounds(a.jitterChart.ClientBoundsPixels())
	if err := drawChartHeader(canvas, headerRect, "One-way jitter per "+periodLabel(a.period)+" (p95)", lastItems(points, "ms", -1, false)); err != nil {
		return err
	}
	return a.drawAggregateChart(canvas, chartRect, points, maxValue, "ms", walk.RGB(215, 160, 70))
}

func (a *app) drawAggregateChart(canvas *walk.Canvas, rect walk.Rectangle, points []chartPoint, maxValue float64, unit string, color walk.Color) error {
	plot := plotBounds(rect)
	visibleLimit := aggregateVisiblePointLimit(points, plot.Width)
	if len(points) > visibleLimit {
		points = points[len(points)-visibleLimit:]
	}
	if len(points) == 0 {
		now := time.Now()
		return drawTimeChart(canvas, rect, nil, now.Add(-a.period), now, 10*time.Minute, 0, maxValue, unit, color)
	}
	start := points[0].at
	end := points[len(points)-1].at
	if !end.After(start) {
		start = end.Add(-a.period)
	}
	return drawTimeChart(canvas, rect, points, start, end, 10*time.Minute, 0, maxValue, unit, color)
}

func aggregateVisiblePointLimit(points []chartPoint, plotWidth int) int {
	if plotWidth < 1 {
		plotWidth = 1
	}
	groups := map[int]struct{}{}
	for _, p := range points {
		groups[p.groupIndex] = struct{}{}
	}
	groupCount := max(1, len(groups))
	return plotWidth * groupCount
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
	if value >= thresholds[2] {
		return 3
	}
	if value >= thresholds[1] {
		return 2
	}
	if value >= thresholds[0] {
		return 1
	}
	return 0
}

func lastItems(points []chartPoint, unit string, special float64, includeGroupName bool) []lastItem {
	if len(points) == 0 {
		return nil
	}
	latest := map[int]chartPoint{}
	for _, p := range points {
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
		if special >= 0 && p.value == special {
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
	lost := r.status != "Success"
	value := r.rtt
	if lost {
		value = lostRTT
		s.lostTotal++
	} else {
		if r.rtt < s.minRTT {
			s.minRTT = r.rtt
		}
		if r.rtt > s.maxRTT {
			s.maxRTT = r.rtt
		}
	}
	s.total++
	s.rtts = append(s.rtts, value)
	keep := max(600, s.aggSeconds*s.pingsPerBatch*2)
	if len(s.rtts) > keep {
		s.rtts = append([]int(nil), s.rtts[len(s.rtts)-keep:]...)
	}
	windowSize := min(len(s.rtts), max(1, s.aggSeconds*s.pingsPerBatch))
	window := s.rtts[len(s.rtts)-windowSize:]
	ok := withoutLost(window)
	p95 := 0.0
	if len(ok) > 0 {
		p95 = statsInts(ok).p95
	}
	lostWindow := 0
	for _, v := range window {
		if v == lostRTT {
			lostWindow++
		}
	}
	minRTT := s.minRTT
	if minRTT == math.MaxInt {
		minRTT = 0
	}
	ev := sampleEvent{
		at:          r.sentAt,
		groupIndex:  s.groupIndex,
		color:       s.color,
		rtt:         value,
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
			groupIndex: s.groupIndex,
			groupName:  s.targetLabel,
			color:      s.color,
			at:         r.sentAt,
			p95:        p95,
			loss:       ev.windowLoss,
			jitterP95:  ev.jitterP95,
		}
		s.lastAgg = r.sentAt
	}
	return ev
}

func pingBatch(ctx context.Context, hosts []string, destination string, sentAt time.Time) pingResult {
	var wg sync.WaitGroup
	ch := make(chan pingResult, len(hosts))
	for _, h := range hosts {
		host := h
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- pingOnce(ctx, host, sentAt)
		}()
	}
	wg.Wait()
	close(ch)
	best := pingResult{sentAt: sentAt, rtt: lostRTT, destination: destination, status: "failure"}
	warnings := make([]string, 0, len(hosts))
	for r := range ch {
		if r.status == "Success" {
			if best.status != "Success" || r.rtt < best.rtt {
				best = r
				best.destination = destination
			}
		} else if r.warning != "" {
			warnings = append(warnings, r.destination+": "+r.warning)
		}
	}
	if best.status == "Success" {
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
	cmd.SysProcAttr = hiddenSysProcAttr()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	text := buf.String()
	if m := reTime.FindStringSubmatch(text); len(m) == 2 {
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

func hiddenSysProcAttr() *syscall.SysProcAttr {
	if runtime.GOOS != "windows" {
		return nil
	}
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
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

func p95Jitter(values []int) float64 {
	if len(values) < 2 {
		return 0
	}
	var jitters []int
	prev := values[0]
	for _, v := range values[1:] {
		if v != lostRTT && prev != lostRTT {
			jitters = append(jitters, int(math.Round(math.Abs(float64(v-prev))/2)))
		}
		prev = v
	}
	if len(jitters) == 0 {
		return 0
	}
	return statsInts(jitters).p95
}

func parseTargets(value string) []string {
	targets := make([]string, 0, 4)
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			targets = append(targets, part)
		}
	}
	return targets
}

func resolveTargets(targets []string, gateway string) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		resolved := resolveTarget(target, gateway)
		if resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

func resolveTarget(target, gateway string) string {
	target = strings.TrimSpace(target)
	switch strings.ToLower(target) {
	case "localhost":
		return "127.0.0.1"
	case "gateway":
		return strings.TrimSpace(gateway)
	default:
		return target
	}
}

func targetListNeedsGateway(targets []string) bool {
	for _, target := range targets {
		if strings.EqualFold(strings.TrimSpace(target), "gateway") {
			return true
		}
	}
	return false
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

func maxChartValue(points []chartPoint, special float64) float64 {
	out := 0.0
	for _, p := range points {
		if special >= 0 && p.value == special {
			continue
		}
		if p.value > out {
			out = p.value
		}
	}
	return out
}

func bucketedYMax(points []chartPoint, buckets []float64, special float64) float64 {
	if len(buckets) == 0 {
		return 1
	}
	value := percentileValue(points, special, 0.90)
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

func percentileValue(points []chartPoint, special float64, percentile float64) float64 {
	values := make([]float64, 0, len(points))
	for _, p := range points {
		if special >= 0 && p.value == special {
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
	maxLoss := maxChartValue(points, -1)
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
	maxJitter := maxChartValue(points, -1)
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
