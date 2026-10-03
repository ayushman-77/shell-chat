package views

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ayushman-77/shell-chat/internal/models"
	"github.com/ayushman-77/shell-chat/internal/storage"
	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

type AdminView struct {
	width       int
	height      int
	userStore   *storage.UserStore
	msgStore    *storage.MessageStore
	user        *models.User
	
	onlineCount int
	totalUsers  int
	topUser     string
	topMsgs     int
	sysHealth   string
	
	kickInput   textinput.Model
	kickMsg     string
}

type AdminMetricsLoadedMsg struct {
	TotalUsers int
	TopUser    string
	TopMsgs    int
	SysHealth  string
}

type AdminKickUserMsg struct {
	Username string
}

func NewAdminView(w, h int, uStore *storage.UserStore, mStore *storage.MessageStore) AdminView {
	ti := textinput.New()
	ti.Placeholder = "Type a username to ban..."
	ti.CharLimit = 32
	ti.Width = 30

	return AdminView{
		width:     w,
		height:    h,
		userStore: uStore,
		msgStore:  mStore,
		topUser:   "None",
		kickInput: ti,
	}
}

type AdminTickMsg struct{}

func AdminTickCmd() tea.Cmd {
	return tea.Tick(time.Second*2, func(time.Time) tea.Msg {
		return AdminTickMsg{}
	})
}

func (v AdminView) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, v.LoadMetricsCmd(), AdminTickCmd())
}

func (v AdminView) SetUser(u *models.User) AdminView {
	v.user = u
	return v
}

func (v AdminView) SetOnlineCount(count int) AdminView {
	v.onlineCount = count
	return v
}

func (v AdminView) LoadMetricsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		totalUsers := 0
		if v.userStore != nil {
			totalUsers = v.userStore.GetTotalUsers(ctx)
		}

		topUser := "None"
		topMsgs := 0
		sysHealth := ""

		// Fetch real analytics from the Python Microservice REST API (using Docker internal DNS)
		resp, err := http.Get("http://python-analytics:8000/api/analytics/top-users")
		if err != nil {
			sysHealth = "⚠️ Partial Outage: Python Analytics Offline"
		} else {
			defer resp.Body.Close()
			var topUsers []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&topUsers); err == nil && len(topUsers) > 0 {
				topUser = "@" + topUsers[0]["username"].(string)
				topMsgs = int(topUsers[0]["messages"].(float64))
			}
		}

		return AdminMetricsLoadedMsg{
			TotalUsers: totalUsers,
			TopUser:    topUser,
			TopMsgs:    topMsgs,
			SysHealth:  sysHealth,
		}
	}
}

func (v AdminView) Update(msg tea.Msg) (AdminView, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter {
			val := strings.TrimSpace(v.kickInput.Value())
			if val != "" {
				v.kickInput.SetValue("")
				return v, func() tea.Msg {
					return AdminKickUserMsg{Username: val}
				}
			}
		}
		v.kickInput, cmd = v.kickInput.Update(msg)
		return v, cmd
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	case AdminMetricsLoadedMsg:
		v.totalUsers = msg.TotalUsers
		v.topUser = msg.TopUser
		v.topMsgs = msg.TopMsgs
		v.sysHealth = msg.SysHealth
	case AdminTickMsg:
		return v, tea.Batch(v.LoadMetricsCmd(), AdminTickCmd())
	}
	
	if !v.kickInput.Focused() {
		v.kickInput.Focus()
	}
	v.kickInput, cmd = v.kickInput.Update(msg)
	return v, cmd
}

func (v AdminView) SetKickMsg(msg string) AdminView {
	v.kickMsg = msg
	return v
}

func (v AdminView) View() string {
	box := lipgloss.NewStyle().
		Width(v.width - 4).
		Height(v.height - 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 2).
		Align(lipgloss.Center)

	title := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("🛡️ ADMIN DASHBOARD")
	subtitle := lipgloss.NewStyle().Foreground(styles.TextMuted).Render("Press F10 to exit")
	header := lipgloss.JoinVertical(lipgloss.Center, title, "\n", subtitle)
	
	// Create stat boxes with a fixed height to ensure alignment
	statBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.SurfaceHover).
		Padding(1, 3).
		Width(24).
		Height(4).
		Align(lipgloss.Center)

	// Box 1: Online Users
	onlineTitle := lipgloss.NewStyle().Foreground(styles.Secondary).Bold(true).Render("ONLINE USERS")
	onlineVal := lipgloss.NewStyle().Foreground(styles.TextBright).Bold(true).Render(fmt.Sprintf("%d", v.onlineCount))
	b1 := statBox.Copy().BorderForeground(styles.Secondary).Render(lipgloss.JoinVertical(lipgloss.Center, onlineTitle, "\n", onlineVal))

	// Box 2: Total Registered
	totalTitle := lipgloss.NewStyle().Foreground(styles.Pink).Bold(true).Render("REGISTERED USERS")
	totalVal := lipgloss.NewStyle().Foreground(styles.TextBright).Bold(true).Render(fmt.Sprintf("%d", v.totalUsers))
	b2 := statBox.Copy().BorderForeground(styles.Pink).Render(lipgloss.JoinVertical(lipgloss.Center, totalTitle, "\n", totalVal))

	// Box 3: Most Active User
	activeTitle := lipgloss.NewStyle().Foreground(styles.Yellow).Bold(true).Render("MOST ACTIVE USER")
	var activeVal string
	if v.topUser == "None" {
		activeVal = lipgloss.NewStyle().Foreground(styles.TextDim).Bold(true).Render("N/A")
	} else {
		activeVal = lipgloss.NewStyle().Foreground(styles.TextBright).Bold(true).Render(fmt.Sprintf("%s", v.topUser))
	}
	b3 := statBox.Copy().BorderForeground(styles.Yellow).Render(lipgloss.JoinVertical(lipgloss.Center, activeTitle, "\n", activeVal))

	boxes := lipgloss.JoinHorizontal(lipgloss.Top, b1, "  ", b2, "  ", b3)

	// Kick User section
	kickTitle := lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("🔨 BAN USER")
	kickArea := lipgloss.JoinVertical(lipgloss.Left, kickTitle, v.kickInput.View())
	if v.kickMsg != "" {
		kickArea = lipgloss.JoinVertical(lipgloss.Left, kickArea, styles.ErrorStyle.Render(v.kickMsg))
	}

	layout := lipgloss.JoinVertical(lipgloss.Center, 
		header, 
		"\n\n", 
		boxes, 
		"\n\n\n",
		lipgloss.NewStyle().Width(76).Align(lipgloss.Left).Render(kickArea),
	)

	return lipgloss.Place(v.width, v.height, lipgloss.Center, lipgloss.Center, box.Render(layout))
}
