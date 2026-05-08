package tui

import (
	"fmt"
	"strings"
	"time"

	"bully/internal/engine"
	"bully/internal/queue"
	"bully/internal/vpn"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	primary = lipgloss.Color("#7c3aed")
	success = lipgloss.Color("#10b981")
	warning = lipgloss.Color("#f59e0b")
	danger  = lipgloss.Color("#ef4444")
	muted   = lipgloss.Color("#6b7280")
	bright  = lipgloss.Color("#e2e8f0")

	// Core styles
	appStyle = lipgloss.NewStyle().Padding(0)

	titleStyle = lipgloss.NewStyle().
			Foreground(primary).
			Bold(true).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(0, 1)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(muted).
			Padding(1, 2).
			Margin(0, 0, 1, 0)

	activeCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primary).
				Padding(1, 2).
				Margin(0, 0, 1, 0)

	helpStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	// Progress bar characters
	barFull  = lipgloss.NewStyle().Foreground(primary).Render("█")
	barEmpty = lipgloss.NewStyle().Foreground(lipgloss.Color("#334155")).Render("█")
)

// Model is the main Bubble Tea model.
type Model struct {
	client   *engine.Client
	queue    *queue.Queue
	vpn      *vpn.Detector
	input    textinput.Model
	width    int
	height   int
	err      error
	lastTick time.Time
	ready    bool
}

// NewModel creates a new TUI model.
func NewModel(client *engine.Client, q *queue.Queue, v *vpn.Detector) *Model {
	ti := textinput.New()
	ti.Placeholder = "Paste magnet link here and press Enter..."
	ti.CharLimit = 2048
	ti.Width = 78
	ti.Focus()
	ti.Prompt = ""

	return &Model{
		client: client,
		queue:  q,
		vpn:    v,
		input:  ti,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tickCmd(),
		vpnTickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func vpnTickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return vpnTickMsg(t)
	})
}

type tickMsg time.Time
type vpnTickMsg time.Time

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			magnet := strings.TrimSpace(m.input.Value())
			if magnet != "" {
				if strings.HasPrefix(magnet, "magnet:") || strings.HasPrefix(magnet, "http") {
					item, err := m.queue.Add(magnet)
					if err != nil {
						m.err = err
					} else {
						m.err = nil
						m.input.SetValue("")
						// If the item was previously failed, reset it and retry
						if item.Status == queue.StatusFailed {
							m.queue.Update(item.MagnetURI, queue.StatusQueued, 0, "", "")
							item.Status = queue.StatusQueued
						}
						if item.Status == queue.StatusQueued && !m.queue.HasActive() {
							m.startDownload(item.MagnetURI)
						}
					}
				} else {
					m.err = fmt.Errorf("not a valid magnet link or .torrent URL")
				}
			}
		}

	case tickMsg:
		m.lastTick = time.Time(msg)
		m.syncFromEngine()
		if !m.queue.HasActive() {
			if next := m.queue.Next(); next != nil {
				m.startDownload(next.MagnetURI)
			}
		}
		cmds = append(cmds, tickCmd())

	case vpnTickMsg:
		m.vpn.Check()
		cmds = append(cmds, vpnTickCmd())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
	}

	inputModel, inputCmd := m.input.Update(msg)
	m.input = inputModel
	cmds = append(cmds, inputCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) startDownload(uri string) {
	m.queue.Update(uri, queue.StatusDownloading, 0, "", "")
	go func() {
		var err error
		if strings.HasPrefix(uri, "magnet:") {
			_, err = m.client.AddMagnet(uri)
		} else {
			_, err = m.client.AddFromURL(uri)
		}
		if err != nil {
			m.queue.Update(uri, queue.StatusFailed, 0, "", err.Error())
		}
	}()
}

func (m *Model) syncFromEngine() {
	for _, info := range m.client.GetAllTorrents() {
		status := queue.Status(string(info.Status))
		name := info.Name
		if name == "" && info.Status == engine.StatusDownloading {
			name = "fetching metadata..."
		}
		errMsg := info.Error
		m.queue.Update(info.MagnetURI, status, info.Progress, name, errMsg)
	}
}

func (m *Model) View() string {
	if !m.ready {
		return "initializing..."
	}

	var b strings.Builder

	// Header with VPN status
	vpnStyle := lipgloss.NewStyle().Foreground(success)
	if !m.vpn.Active() {
		vpnStyle = lipgloss.NewStyle().Foreground(danger)
	}
	b.WriteString(titleStyle.Render("⚡ bully"))
	b.WriteString(headerStyle.Render("zero-config torrent downloader"))
	b.WriteString(vpnStyle.Render("  " + m.vpn.Status()))
	b.WriteString("\n\n")

	// VPN warning banner
	if !m.vpn.Active() {
		banner := lipgloss.NewStyle().
			Foreground(warning).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(warning).
			Padding(1, 2).
			Width(m.width - 4).
			Render("⚠️  No VPN detected! Your IP address and download activity are visible to peers. Connect to a VPN before downloading.")
		b.WriteString(banner)
		b.WriteString("\n\n")
	}

	// Input
	b.WriteString(inputStyle.Render(m.input.View()))
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(danger).Padding(0, 1).Render("  ⚠ " + m.err.Error()))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Downloads
	items := m.queue.All()
	if len(items) == 0 {
		b.WriteString(mutedStyle("  No downloads yet. Paste a magnet link above to get started.\n"))
	} else {
		for _, item := range items {
			b.WriteString(m.renderCard(item))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  enter: add download  •  ctrl+c/q: quit  •  downloads → ~/Downloads/bully/"))
	b.WriteString("\n")

	return appStyle.Render(b.String())
}

func (m *Model) renderCard(item *queue.Item) string {
	style := cardStyle
	if item.Status == queue.StatusDownloading {
		style = activeCardStyle
	}

	cardWidth := m.width - 4
	if cardWidth < 40 {
		cardWidth = 40
	}

	var content strings.Builder

	// Row 1: status icon + name
	statusIcon, statusColor := statusIndicator(item.Status)
	name := item.Name
	if name == "" {
		name = truncateMagnet(item.MagnetURI, 50)
	}

	row1 := lipgloss.NewStyle().Foreground(statusColor).Render(statusIcon)
	row1 += " " + lipgloss.NewStyle().Foreground(bright).Bold(true).Render(name)
	content.WriteString(row1)
	content.WriteString("\n")

	// Row 2: progress bar + percentage (if downloading/seeding)
	if item.Status == queue.StatusDownloading {
		barWidth := cardWidth - 15
		if barWidth < 10 {
			barWidth = 10
		}
		bar := renderBar(item.Progress, barWidth)
		pct := fmt.Sprintf(" %5.1f%%", item.Progress*100)
		content.WriteString("  " + bar + lipgloss.NewStyle().Foreground(primary).Render(pct))
		content.WriteString("\n")
	}

	// Row 3: details line
	content.WriteString("  " + mutedStyle(m.detailLine(item)))

	return style.Width(cardWidth).Render(content.String())
}

func statusIndicator(status queue.Status) (string, lipgloss.Color) {
	switch status {
	case queue.StatusQueued:
		return "⏳", muted
	case queue.StatusDownloading:
		return "⬇", primary
	case queue.StatusCompleted:
		return "✅", success
	case queue.StatusFailed:
		return "❌", danger
	default:
		return "•", muted
	}
}

func (m *Model) detailLine(item *queue.Item) string {
	// Use engine data when available for live stats
	info := m.client.GetTorrent(item.MagnetURI)

	switch item.Status {
	case queue.StatusDownloading:
		parts := []string{}
		if info != nil && info.SpeedDown > 0 {
			parts = append(parts, fmtSpeed(info.SpeedDown)+"/s")
		}
		if info != nil && info.Peers > 0 {
			parts = append(parts, fmt.Sprintf("%d peers", info.Peers))
		}
		if info != nil && info.Seeds > 0 {
			parts = append(parts, fmt.Sprintf("%d seeds", info.Seeds))
		}
		if info != nil && info.ETA > 0 && info.ETA < 365*24*time.Hour {
			parts = append(parts, "eta "+fmtDuration(info.ETA))
		}
		if len(parts) == 0 {
			parts = append(parts, "connecting...")
		}
		return strings.Join(parts, "  •  ")

	case queue.StatusCompleted:
		return "download finished " + friendlyTime(item.CompletedAt)

	case queue.StatusQueued:
		// Show position in queue
		for i, qi := range m.queue.All() {
			if qi.MagnetURI == item.MagnetURI {
				return fmt.Sprintf("queued (#%d)", i+1)
			}
		}
		return "queued"

	case queue.StatusFailed:
		if item.Error != "" {
			return "error: " + item.Error
		}
		return "download failed"

	default:
		return ""
	}
}

func renderBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	filled := int(progress * float64(width))
	if filled > width {
		filled = width
	}

	var bar strings.Builder
	for i := 0; i < filled; i++ {
		bar.WriteString(barFull)
	}
	for i := filled; i < width; i++ {
		bar.WriteString(barEmpty)
	}
	return bar.String()
}

func fmtSpeed(bytesPerSec int64) string {
	switch {
	case bytesPerSec >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytesPerSec)/(1<<30))
	case bytesPerSec >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytesPerSec)/(1<<20))
	case bytesPerSec >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytesPerSec)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytesPerSec)
	}
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d >= 24*time.Hour:
		days := d / (24 * time.Hour)
		remainder := d % (24 * time.Hour)
		return fmt.Sprintf("%dd%dh", days, remainder/time.Hour)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", d/time.Hour, (d%time.Hour)/time.Minute)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%ds", d/time.Minute, (d%time.Minute)/time.Second)
	default:
		return fmt.Sprintf("%ds", d/time.Second)
	}
}

func truncateMagnet(magnet string, maxLen int) string {
	if len(magnet) <= maxLen {
		return magnet
	}
	return magnet[:maxLen-3] + "..."
}

func mutedStyle(s string) string {
	return lipgloss.NewStyle().Foreground(muted).Render(s)
}

func friendlyTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	d := time.Since(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
