package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"

	"github.com/ayushman-77/shell-chat/internal/actor"
	"github.com/ayushman-77/shell-chat/internal/bot"
	"github.com/ayushman-77/shell-chat/internal/calc"
	"github.com/ayushman-77/shell-chat/internal/coalescer"
	"github.com/ayushman-77/shell-chat/internal/models"
	"github.com/ayushman-77/shell-chat/internal/pubsub"
	sfgen "github.com/ayushman-77/shell-chat/internal/snowflake"
	"github.com/ayushman-77/shell-chat/internal/storage"
	"github.com/ayushman-77/shell-chat/internal/tui/components"
	"github.com/ayushman-77/shell-chat/internal/tui/styles"
	"github.com/ayushman-77/shell-chat/internal/tui/views"
)

// ViewState tracks the active application view.
type ViewState int

const (
	ViewLogin ViewState = iota
	ViewChat
)

// FocusArea tracks which panel has keyboard focus in the chat view.
type FocusArea int

const (
	FocusSidebarArea FocusArea = iota
	FocusChatArea
	FocusInputArea
	FocusRightSidebar
)

// --- Async result messages ---

type GuildsLoadedMsg struct{ Guilds []*models.Guild }
type ChannelsLoadedMsg struct{ Channels []*models.Channel }
type MessagesLoadedMsg struct{ Messages []*models.Message }
type ErrorMsg struct{ Err error }

// App is the root BubbleTea model.
type App struct {
	width  int
	height int

	viewState ViewState
	focus     FocusArea

	username  string
	user      *models.User
	sessionID string

	loginView   views.LoginView
	sidebar     views.Sidebar
	msgView     views.MessageView
	membersView views.MembersView
	input       views.Input
	statusBar   components.StatusBar
	modal       components.Modal

	registry   *actor.Registry
	userStore  *storage.UserStore
	guildStore *storage.GuildStore
	msgStore   *storage.MessageStore
	coalescer  *coalescer.Coalescer
	broker     *pubsub.Broker
	subscriber *pubsub.SessionSubscriber
	logger     *log.Logger

	sessionActor *actor.SessionActor
	program      *tea.Program

	currentGuild    *models.Guild
	currentChannel  *models.Channel
	currentDMUserID int64
	guilds          []*models.Guild
	channels        []*models.Channel
	location        *time.Location
	geminiAPIKey    string
	geminiModel     string

	err error
}

func NewApp(
	username string,
	width, height int,
	registry *actor.Registry,
	userStore *storage.UserStore,
	guildStore *storage.GuildStore,
	msgStore *storage.MessageStore,
	c *coalescer.Coalescer,
	broker *pubsub.Broker,
	logger *log.Logger,
) *App {
	sessionID := fmt.Sprintf("sess_%s_%d", username, time.Now().UnixNano())

	if c == nil {
		c = coalescer.NewCoalescer(msgStore, guildStore, userStore)
	}

	loc := DefaultLocation()

	return &App{
		width:       width,
		height:      height,
		viewState:   ViewLogin,
		focus:       FocusInputArea,
		username:    username,
		sessionID:   sessionID,
		loginView:   views.NewLoginView(username, userStore),
		sidebar:     views.NewSidebar(),
		msgView:     views.NewMessageView(width-25, height-4).SetLocation(loc),
		membersView: views.NewMembersView(20, height-1),
		input:       views.NewInput(),
		statusBar:   components.NewStatusBar(username),
		modal:       components.NewModal(),
		registry:    registry,
		userStore:   userStore,
		guildStore:  guildStore,
		msgStore:    msgStore,
		coalescer:   c,
		broker:      broker,
		logger:      logger,
		location:    loc,
	}
}

// SetAIConfig sets the Gemini API key and model for Spark AI.
func (a *App) SetAIConfig(geminiKey, geminiModel string) {
	a.geminiAPIKey = geminiKey
	a.geminiModel = geminiModel
}

// SetProgram sets the BubbleTea program reference for actor messaging.
func (a *App) SetProgram(p *tea.Program) {
	a.program = p
	if a.sessionActor != nil {
		a.sessionActor.SetProgram(p)
	}
}

func (a *App) Init() tea.Cmd {
	return a.loginView.Init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeAll()
		return a, nil

	case tea.KeyMsg:
		if key.Matches(msg, Keys.Quit) {
			a.cleanup()
			return a, tea.Quit
		}
		if a.modal.IsVisible() {
			var cmd tea.Cmd
			a.modal, cmd = a.modal.Update(msg)
			if !a.modal.IsVisible() {
				a.focus = FocusInputArea
				a.input = a.input.Focus()
			}
			return a, cmd
		}
		switch a.viewState {
		case ViewLogin:
			var cmd tea.Cmd
			a.loginView, cmd = a.loginView.Update(msg)
			return a, cmd
		case ViewChat:
			return a.updateChatKeys(msg)
		}

	case views.LoginSuccessMsg:
		a.user = msg.User
		a.viewState = ViewChat
		a.focus = FocusInputArea
		a.input = a.input.Focus()

		displayName := a.user.DisplayName
		if displayName == "" {
			displayName = a.user.Username
		}
		a.statusBar = a.statusBar.SetUsername(displayName).SetConnected(true)
		a.msgView = a.msgView.SetUsername(a.user.ID, displayName)

		// 1. Initialize and register SessionActor
		a.sessionActor = actor.NewSessionActor(a.user.ID, a.user.Username, a.sessionID, a.program, a.registry)
		sessionRef := actor.NewRef("session:"+a.sessionID, a.sessionActor, 512, a.logger)
		a.registry.Register(sessionRef)

		// Broadcast new online user list to all connected sessions
		a.registry.BroadcastOnlineUsers()

		// Initial load of online users
		online := a.registry.GetOnlineUsers()
		a.membersView = a.membersView.SetUsers(online)
		a.statusBar = a.statusBar.SetMemberCount(len(online))

		// Load past DM conversation partners from storage
		if a.msgStore != nil {
			partners := a.msgStore.GetDMPartners(a.user.ID)
			ctx := context.Background()
			for _, pid := range partners {
				pName := fmt.Sprintf("user_%d", pid)
				if a.userStore != nil {
					if u, err := a.userStore.GetUserByID(ctx, pid); err == nil && u != nil {
						pName = u.Username
					}
				}
				a.membersView = a.membersView.AddOrUpdateDM(pid, pName, false)
			}
		}

		// 2. Initialize SessionSubscriber for Pub/Sub
		a.subscriber = pubsub.NewSessionSubscriber(a.broker, a.sessionID, a.registry, a.logger)

		// Post join announcement to #announcements
		annMsgID := sfgen.Generate()
		annMsg := &models.Message{
			ID:         annMsgID,
			ChannelID:  storage.DefaultAnnouncementsChannelID,
			Bucket:     models.BucketFromSnowflake(annMsgID),
			AuthorID:   models.SparkBotID,
			AuthorName: "📢 System",
			Content:    fmt.Sprintf("📢 @%s has joined the server", a.user.Username),
			CreatedAt:  models.TimeFromSnowflake(annMsgID),
		}
		chRef := a.registry.GetOrCreateChannelActor(
			storage.DefaultAnnouncementsChannelID,
			storage.DefaultCommunityGuildID,
			a.msgStore,
			a.broker,
			a.logger,
		)
		chRef.Send(actor.PostMessage{Msg: annMsg})

		a.resizeAll()
		return a, tea.Batch(a.waitForActorMsg(), a.loadGuilds())

	case views.LoginErrorMsg:
		a.loginView = a.loginView.SetError(msg.Err.Error())
		return a, nil

	case GuildsLoadedMsg:
		a.guilds = msg.Guilds
		a.sidebar = a.sidebar.SetGuilds(msg.Guilds)
		if len(msg.Guilds) > 0 {
			a.currentGuild = msg.Guilds[0]
			a.statusBar = a.statusBar.SetGuild(a.currentGuild.Name)
			return a, a.loadChannels(a.currentGuild.ID)
		}
		return a, nil

	case ChannelsLoadedMsg:
		a.channels = msg.Channels
		a.sidebar = a.sidebar.SetChannels(msg.Channels)

		// Join and subscribe to ALL guild channels so real-time unread badges work across all channels!
		if a.user != nil {
			for _, ch := range msg.Channels {
				guildID := ch.GuildID
				if guildID == 0 && a.currentGuild != nil {
					guildID = a.currentGuild.ID
				}
				channelRef := a.registry.GetOrCreateChannelActor(ch.ID, guildID, a.msgStore, a.broker, a.logger)
				channelRef.Send(actor.JoinChannel{
					UserID:    a.user.ID,
					SessionID: a.sessionID,
				})
				if a.subscriber != nil && guildID != 0 {
					_ = a.subscriber.SubscribeChannel(context.Background(), guildID, ch.ID)
				}
			}
		}

		if len(msg.Channels) > 0 && a.currentChannel == nil {
			target := msg.Channels[0]
			for _, ch := range msg.Channels {
				if ch.Name == "general" {
					target = ch
					break
				}
			}
			return a, a.selectChannel(target)
		}
		return a, nil

	case MessagesLoadedMsg:
		for _, m := range msg.Messages {
			if m.AuthorName != "" {
				a.msgView = a.msgView.SetUsername(m.AuthorID, m.AuthorName)
			}
		}
		a.msgView = a.msgView.SetMessages(msg.Messages)

	case actor.ChatMsg:
		if msg.Message.AuthorName != "" {
			a.msgView = a.msgView.SetUsername(msg.Message.AuthorID, msg.Message.AuthorName)
		}

		if a.currentChannel != nil && msg.Message.ChannelID == a.currentChannel.ID {
			// Currently viewing this channel / DM -> display immediately
			a.msgView = a.msgView.AddMessage(msg.Message)
		} else {
			// Message received for another channel or DM!
			isDM := false
			if a.user != nil && msg.Message.AuthorID != a.user.ID {
				if msg.Message.ChannelID == models.DMChannelID(a.user.ID, msg.Message.AuthorID) {
					isDM = true
				}
			}

			if isDM {
				// 1-on-1 DM: show blue dot in DIRECT MESSAGES on right sidebar
				authorName := msg.Message.AuthorName
				if authorName == "" {
					authorName = fmt.Sprintf("user_%d", msg.Message.AuthorID)
				}
				if a.msgStore != nil {
					a.msgStore.RecordDMPartner(a.user.ID, msg.Message.AuthorID)
				}
				a.membersView = a.membersView.AddOrUpdateDM(msg.Message.AuthorID, authorName, true)
			} else {
				// Server channel (#general, #dev): show blue dot beside that channel in left sidebar
				a.sidebar = a.sidebar.MarkUnread(msg.Message.ChannelID)
			}
		}
		return a, a.waitForActorMsg()

	case actor.MembersMsg:
		a.membersView = a.membersView.SetUsers(msg.Users)
		a.statusBar = a.statusBar.SetMemberCount(len(msg.Users))
		return a, a.waitForActorMsg()

	case actor.TypingMsg:
		if a.currentChannel != nil && msg.ChannelID == a.currentChannel.ID {
			a.msgView = a.msgView.AddTyping(msg.Username)
		}
		return a, a.waitForActorMsg()

	case actor.SystemMsg:
		a.msgView = a.msgView.AddSystemMessage(msg.Content)
		return a, a.waitForActorMsg()

	case SparkResponseMsg:
		botMsgID := sfgen.Generate()
		botMsg := &models.Message{
			ID:         botMsgID,
			ChannelID:  msg.ChannelID,
			Bucket:     models.BucketFromSnowflake(botMsgID),
			AuthorID:   models.SparkBotID,
			AuthorName: models.SparkBotName,
			Content:    msg.Answer,
			CreatedAt:  models.TimeFromSnowflake(botMsgID),
		}

		if a.currentChannel != nil && a.currentChannel.ID == msg.ChannelID {
			a.msgView = a.msgView.AddMessage(botMsg)
		}

		// Broadcast message to channel / DM actor so all members see Spark's response!
		channelRef := a.registry.GetOrCreateChannelActor(msg.ChannelID, msg.GuildID, a.msgStore, a.broker, a.logger)
		channelRef.Send(actor.PostMessage{
			Msg:          botMsg,
			TargetUserID: msg.TargetDMUserID,
		})
		return a, nil

	case views.DMSelectedMsg:
		if a.user != nil {
			targetID := msg.UserID
			targetName := msg.Username
			a.currentDMUserID = targetID
			dmChannelID := models.DMChannelID(a.user.ID, targetID)
			a.currentChannel = &models.Channel{
				ID:      dmChannelID,
				GuildID: 0,
				Name:    "@" + targetName,
				Topic:   "Direct Message with " + targetName,
			}
			if a.msgStore != nil {
				a.msgStore.RecordDMPartner(a.user.ID, targetID)
			}
			a.sidebar = a.sidebar.SetActiveChannel(0)
			a.membersView = a.membersView.SetActiveDM(targetID).AddOrUpdateDM(targetID, targetName, false).ClearUnread(targetID)
			a.statusBar = a.statusBar.SetChannel("@" + targetName)
			a.msgView = a.msgView.SetChannel("@"+targetName, "Direct Message with "+targetName)
			a.input = a.input.SetDisabled(false).SetPlaceholder("Message @" + targetName).Focus()
			a.focus = FocusInputArea
			a.sidebar = a.sidebar.SetFocused(false)
			a.membersView = a.membersView.SetFocused(false)

			if a.sessionActor != nil {
				a.sessionActor.SetActiveContext(0, dmChannelID)
			}

			channelRef := a.registry.GetOrCreateChannelActor(dmChannelID, 0, a.msgStore, a.broker, a.logger)
			channelRef.Send(actor.JoinChannel{
				UserID:    a.user.ID,
				SessionID: a.sessionID,
			})
			return a, a.loadMessages(dmChannelID)
		}

	case views.SendMessageMsg:
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return a, nil
		}

		// Block message sending in read-only announcement channels
		if a.currentChannel != nil && a.currentChannel.Type == models.ChannelTypeAnnouncement {
			a.msgView = a.msgView.AddSystemMessage("🔒 #announcements is a read-only channel.")
			return a, nil
		}

		// Handle /settings command
		if strings.HasPrefix(content, "/settings") {
			if a.user != nil {
				a.modal = a.modal.ShowSettings(a.user.Username)
				return a, nil
			}
		}

		// Handle /help command
		if strings.HasPrefix(content, "/help") || content == "/?" {
			a.modal = a.modal.Show(components.ModalHelp, "Shell Chat Guide")
			return a, nil
		}

		// Handle /tz or /timezone command
		if strings.HasPrefix(content, "/tz") || strings.HasPrefix(content, "/timezone") {
			parts := strings.Fields(content)
			if len(parts) == 1 {
				zoneName, offset := time.Now().In(a.location).Zone()
				offsetHours := float64(offset) / 3600.0
				a.msgView = a.msgView.AddSystemMessage(fmt.Sprintf("Current timezone: %s (UTC%+0.1fh, 24h format). Change with /tz <name/offset> (e.g. /tz IST, /tz +5:30, /tz EST, /tz UTC, /tz Asia/Tokyo)", zoneName, offsetHours))
				return a, nil
			}
			newLoc, err := ParseTimezone(parts[1])
			if err != nil {
				a.msgView = a.msgView.AddSystemMessage(fmt.Sprintf("❌ Error: %s. Examples: /tz IST, /tz +5:30, /tz -5, /tz UTC", err.Error()))
				return a, nil
			}
			a.location = newLoc
			a.msgView = a.msgView.SetLocation(newLoc)
			zoneName, offset := time.Now().In(a.location).Zone()
			a.msgView = a.msgView.AddSystemMessage(fmt.Sprintf("✅ Timezone set to %s (UTC%+0.1fh). 24h local time is active!", zoneName, float64(offset)/3600.0))
			return a, nil
		}

		// Handle /calc command (evaluated and responded by Spark Bot)
		if strings.HasPrefix(content, "/calc") {
			expr := strings.TrimSpace(strings.TrimPrefix(content, "/calc"))
			if expr == "" {
				a.msgView = a.msgView.AddSystemMessage("Usage: /calc <expression> — Fast math calculator (e.g. /calc (1024 * 768) / 8, /calc sqrt(144) + 2^8, /calc sin(pi/2))")
				return a, nil
			}

			if a.currentChannel != nil && a.user != nil {
				// 1. Post user's message so everyone in the channel sees the question
				userMsgID := sfgen.Generate()
				authorName := a.user.DisplayName
				if authorName == "" {
					authorName = a.user.Username
				}
				userMsg := &models.Message{
					ID:         userMsgID,
					ChannelID:  a.currentChannel.ID,
					Bucket:     models.BucketFromSnowflake(userMsgID),
					AuthorID:   a.user.ID,
					AuthorName: authorName,
					Content:    content,
					CreatedAt:  models.TimeFromSnowflake(userMsgID),
				}
				a.msgView = a.msgView.AddMessage(userMsg)
				cmds = append(cmds, a.sendExistingMessage(userMsg))

				// 2. Evaluate math expression
				var botAnswer string
				result, err := calc.Eval(expr)
				if err != nil {
					botAnswer = fmt.Sprintf("❌ Calculation error: %s", err.Error())
				} else {
					resultStr := calc.FormatResult(result)
					botAnswer = fmt.Sprintf("%s = %s", expr, resultStr)
				}

				// Record calculation in Spark's group memory for this channel!
				bot.AddMemory(a.currentChannel.ID, a.user.ID, a.user.Username, content, botAnswer)

				// 3. Post Spark Bot response
				time.Sleep(2 * time.Millisecond) // Guarantee monotonic timestamp/snowflake
				botMsgID := sfgen.Generate()
				botMsg := &models.Message{
					ID:         botMsgID,
					ChannelID:  a.currentChannel.ID,
					Bucket:     models.BucketFromSnowflake(botMsgID),
					AuthorID:   models.SparkBotID,
					AuthorName: models.SparkBotName,
					Content:    botAnswer,
					CreatedAt:  models.TimeFromSnowflake(botMsgID),
				}
				a.msgView = a.msgView.AddMessage(botMsg)

				// Send both sequentially to actors so arrival order is 100% deterministic across all clients
				return a, a.sendMessagesSequential(userMsg, botMsg)
			}
			return a, nil
		}

		// Handle /search command
		if strings.HasPrefix(content, "/search") {
			query := strings.TrimSpace(strings.TrimPrefix(content, "/search"))
			if query == "" {
				a.msgView = a.msgView.AddSystemMessage("Usage: /search <keyword> — Search past message history in this channel/DM.")
				return a, nil
			}
			if a.msgStore == nil || a.currentChannel == nil {
				a.msgView = a.msgView.AddSystemMessage("🔍 No messages found.")
				return a, nil
			}
			ctx := context.Background()
			matches := a.msgStore.SearchMessages(ctx, a.currentChannel.ID, query, 15)
			if len(matches) == 0 {
				a.msgView = a.msgView.AddSystemMessage(fmt.Sprintf("🔍 No messages found matching \"%s\".", query))
				return a, nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🔍 Found %d message(s) matching \"%s\":\n", len(matches), query))
			for i, m := range matches {
				author := m.AuthorName
				if author == "" {
					author = fmt.Sprintf("User #%d", m.AuthorID)
				}
				timeStr := m.CreatedAt.In(a.location).Format("15:04")
				snippet := m.Content
				if len(snippet) > 80 {
					snippet = snippet[:77] + "..."
				}
				sb.WriteString(fmt.Sprintf("  %d. [%s] %s: %s", i+1, timeStr, author, snippet))
				if i < len(matches)-1 {
					sb.WriteString("\n")
				}
			}
			a.msgView = a.msgView.AddSystemMessage(sb.String())
			return a, nil
		}

		// Handle /ask command for Spark (🤖) AI Bot
		if strings.HasPrefix(content, "/ask") {
			prompt := strings.TrimSpace(strings.TrimPrefix(content, "/ask"))
			if prompt == "" {
				a.msgView = a.msgView.AddSystemMessage("Usage: /ask <question> — Ask Spark anything (e.g. /ask what is the capital of Russia?)")
				return a, nil
			}

			if a.currentChannel != nil && a.user != nil {
				// Check per-user cooldown (anti-spam)
				canAsk, remaining := bot.CheckCooldown(a.user.ID)
				if !canAsk {
					a.msgView = a.msgView.AddSystemMessage(fmt.Sprintf("⏳ Please wait %d seconds before asking Spark another question.", remaining))
					return a, nil
				}

				// Post user's question so everyone in the channel sees what was asked
				msgID := sfgen.Generate()
				authorName := a.user.DisplayName
				if authorName == "" {
					authorName = a.user.Username
				}
				userMsg := &models.Message{
					ID:         msgID,
					ChannelID:  a.currentChannel.ID,
					Bucket:     models.BucketFromSnowflake(msgID),
					AuthorID:   a.user.ID,
					AuthorName: authorName,
					Content:    content,
					CreatedAt:  models.TimeFromSnowflake(msgID),
				}
				a.msgView = a.msgView.AddMessage(userMsg)
				cmds = append(cmds, a.sendExistingMessage(userMsg))

				// Show typing indicator: "Spark is thinking..."
				a.msgView = a.msgView.AddTyping("Spark")

				// Launch async background query to Groq with user context
				targetDMUserID := a.currentDMUserID
				channelID := a.currentChannel.ID
				guildID := int64(0)
				if a.currentGuild != nil {
					guildID = a.currentGuild.ID
				}
				if a.currentChannel.GuildID != 0 {
					guildID = a.currentChannel.GuildID
				}

				userName := a.user.DisplayName
				if userName == "" {
					userName = a.user.Username
				}

				cmds = append(cmds, a.querySparkBot(prompt, a.user.ID, userName, channelID, guildID, targetDMUserID))
				return a, tea.Batch(cmds...)
			}
			return a, nil
		}

		if a.currentChannel != nil && a.user != nil {
			msgID := sfgen.Generate()
			authorName := a.user.DisplayName
			if authorName == "" {
				authorName = a.user.Username
			}
			msg := &models.Message{
				ID:         msgID,
				ChannelID:  a.currentChannel.ID,
				Bucket:     models.BucketFromSnowflake(msgID),
				AuthorID:   a.user.ID,
				AuthorName: authorName,
				Content:    content,
				CreatedAt:  models.TimeFromSnowflake(msgID),
			}
			a.msgView = a.msgView.AddMessage(msg)
			cmds = append(cmds, a.sendExistingMessage(msg))
		}

	case views.FocusInputMsg:
		a.focus = FocusInputArea
		a.sidebar = a.sidebar.SetFocused(false)
		a.membersView = a.membersView.SetFocused(false)
		a.input = a.input.Focus()

	case views.ChannelSelectedMsg:
		a.currentDMUserID = 0
		a.membersView = a.membersView.SetActiveDM(0)
		if msg.Guild != nil && (a.currentGuild == nil || a.currentGuild.ID != msg.Guild.ID) {
			a.currentGuild = msg.Guild
			a.statusBar = a.statusBar.SetGuild(msg.Guild.Name)
		}
		a.focus = FocusInputArea
		a.sidebar = a.sidebar.SetFocused(false)
		a.membersView = a.membersView.SetFocused(false)
		a.input = a.input.Focus()
		cmds = append(cmds, a.selectChannel(msg.Channel))

	case views.GuildSelectedMsg:
		a.currentGuild = msg.Guild
		a.statusBar = a.statusBar.SetGuild(msg.Guild.Name)
		cmds = append(cmds, a.loadChannels(msg.Guild.ID))

	case components.CreateGuildMsg:
		cmds = append(cmds, a.createGuild(msg.Name))

	case components.CreateChannelMsg:
		cmds = append(cmds, a.createChannel(msg.Name))

	case components.SaveSettingsMsg:
		if a.user == nil {
			return a, nil
		}
		if strings.TrimSpace(msg.CurrentPassword) == "" {
			a.modal = a.modal.SetError("Current password required to verify identity")
			return a, nil
		}
		if !a.userStore.VerifyPassword(a.user.PasswordHash, msg.CurrentPassword) {
			a.modal = a.modal.SetError("Incorrect current password")
			return a, nil
		}

		ctx := context.Background()
		oldUsername := a.user.Username
		newUsername := strings.TrimSpace(msg.NewUsername)
		changedUsername := false

		if newUsername != "" && newUsername != oldUsername {
			if len(newUsername) < 3 {
				a.modal = a.modal.SetError("Username must be at least 3 characters")
				return a, nil
			}
			exists, err := a.userStore.UsernameExists(ctx, newUsername)
			if err != nil {
				a.modal = a.modal.SetError(fmt.Sprintf("Error: %s", err.Error()))
				return a, nil
			}
			if exists {
				a.modal = a.modal.SetError("Username is already taken")
				return a, nil
			}

			if err := a.userStore.UpdateUsername(ctx, a.user.ID, oldUsername, newUsername); err != nil {
				a.modal = a.modal.SetError(fmt.Sprintf("Update failed: %s", err.Error()))
				return a, nil
			}
			_ = a.msgStore.UpdateAuthorName(ctx, a.user.ID, newUsername)
			changedUsername = true
		}

		if msg.NewPassword != "" {
			if len(msg.NewPassword) < 4 {
				a.modal = a.modal.SetError("New password must be at least 4 characters")
				return a, nil
			}
			if err := a.userStore.UpdatePassword(ctx, a.user.ID, msg.NewPassword); err != nil {
				a.modal = a.modal.SetError(fmt.Sprintf("Password update failed: %s", err.Error()))
				return a, nil
			}
			if updatedUser, err := a.userStore.GetUserByID(ctx, a.user.ID); err == nil && updatedUser != nil {
				a.user.PasswordHash = updatedUser.PasswordHash
			}
		}

		if changedUsername {
			a.user.Username = newUsername
			a.user.DisplayName = newUsername
			a.statusBar = a.statusBar.SetUsername(newUsername)
			a.msgView = a.msgView.UpdateUsername(a.user.ID, newUsername)
			if a.currentChannel == nil || a.currentChannel.ID != storage.DefaultAnnouncementsChannelID {
				a.sidebar = a.sidebar.MarkUnread(storage.DefaultAnnouncementsChannelID)
			}

			// Broadcast to all active sessions across server
			a.registry.BroadcastProfileUpdate(a.user.ID, oldUsername, newUsername)
			a.registry.BroadcastOnlineUsers()

			// Broadcast announcement to #announcements
			annMsgID := sfgen.Generate()
			annMsg := &models.Message{
				ID:         annMsgID,
				ChannelID:  storage.DefaultAnnouncementsChannelID,
				Bucket:     models.BucketFromSnowflake(annMsgID),
				AuthorID:   models.SparkBotID,
				AuthorName: "📢 System",
				Content:    fmt.Sprintf("📢 **@%s** has changed their username to **@%s**", oldUsername, newUsername),
				CreatedAt:  models.TimeFromSnowflake(annMsgID),
			}
			chRef := a.registry.GetOrCreateChannelActor(
				storage.DefaultAnnouncementsChannelID,
				storage.DefaultCommunityGuildID,
				a.msgStore,
				a.broker,
				a.logger,
			)
			chRef.Send(actor.PostMessage{Msg: annMsg})
		}

		a.modal = a.modal.Hide()
		a.focus = FocusInputArea
		a.input = a.input.Focus()
		a.msgView = a.msgView.AddSystemMessage("✅ Settings updated successfully!")
		return a, nil

	case actor.ProfileUpdatedMsg:
		a.msgView = a.msgView.UpdateUsername(msg.UserID, msg.NewUsername)
		if a.user != nil && a.user.ID == msg.UserID {
			a.user.Username = msg.NewUsername
			a.user.DisplayName = msg.NewUsername
			a.statusBar = a.statusBar.SetUsername(msg.NewUsername)
		}
		a.membersView = a.membersView.SetUsers(a.registry.GetOnlineUsers()).UpdateUsername(msg.UserID, msg.NewUsername)
		if a.currentChannel == nil || a.currentChannel.ID != storage.DefaultAnnouncementsChannelID {
			a.sidebar = a.sidebar.MarkUnread(storage.DefaultAnnouncementsChannelID)
		}
		return a, a.waitForActorMsg()

	case ErrorMsg:
		a.err = msg.Err
		return a, a.waitForActorMsg()
	}

	return a, tea.Batch(cmds...)
}

func (a *App) updateChatKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Tab cycles: Input -> Channels (Left) -> Chat History (Scroll) -> Members/DMs (Right) -> Input
	if key.Matches(msg, Keys.ToggleFocus) {
		showRight := a.width >= 80
		switch a.focus {
		case FocusInputArea:
			a.focus = FocusSidebarArea
			a.sidebar = a.sidebar.SetFocused(true)
			a.msgView = a.msgView.SetFocused(false)
			a.membersView = a.membersView.SetFocused(false)
			a.input = a.input.Blur()
		case FocusSidebarArea:
			a.focus = FocusChatArea
			a.sidebar = a.sidebar.SetFocused(false)
			a.msgView = a.msgView.SetFocused(true)
			a.membersView = a.membersView.SetFocused(false)
			a.input = a.input.Blur()
		case FocusChatArea:
			if showRight {
				a.focus = FocusRightSidebar
				a.sidebar = a.sidebar.SetFocused(false)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(true)
				a.input = a.input.Blur()
			} else if a.input.IsDisabled() {
				a.focus = FocusSidebarArea
				a.sidebar = a.sidebar.SetFocused(true)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(false)
				a.input = a.input.Blur()
			} else {
				a.focus = FocusInputArea
				a.sidebar = a.sidebar.SetFocused(false)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(false)
				a.input = a.input.Focus()
			}
		case FocusRightSidebar:
			if a.input.IsDisabled() {
				a.focus = FocusSidebarArea
				a.sidebar = a.sidebar.SetFocused(true)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(false)
				a.input = a.input.Blur()
			} else {
				a.focus = FocusInputArea
				a.sidebar = a.sidebar.SetFocused(false)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(false)
				a.input = a.input.Focus()
			}
		default:
			if a.input.IsDisabled() {
				a.focus = FocusSidebarArea
				a.sidebar = a.sidebar.SetFocused(true)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(false)
				a.input = a.input.Blur()
			} else {
				a.focus = FocusInputArea
				a.sidebar = a.sidebar.SetFocused(false)
				a.msgView = a.msgView.SetFocused(false)
				a.membersView = a.membersView.SetFocused(false)
				a.input = a.input.Focus()
			}
		}
		return a, nil
	}

	// Esc returns focus to input box (or sidebar if read-only) & resets scroll position to bottom
	if key.Matches(msg, Keys.Escape) {
		if a.input.IsDisabled() {
			a.focus = FocusSidebarArea
			a.sidebar = a.sidebar.SetFocused(true)
			a.msgView = a.msgView.SetFocused(false).GotoBottom()
			a.membersView = a.membersView.SetFocused(false)
			a.input = a.input.Blur()
		} else {
			a.focus = FocusInputArea
			a.sidebar = a.sidebar.SetFocused(false)
			a.msgView = a.msgView.SetFocused(false).GotoBottom()
			a.membersView = a.membersView.SetFocused(false)
			a.input = a.input.Focus()
		}
		return a, nil
	}

	// Global scroll shortcuts (always active)
	if key.Matches(msg, Keys.ScrollUp) {
		a.msgView = a.msgView.LineUp(4)
		return a, nil
	}
	if key.Matches(msg, Keys.ScrollDown) {
		a.msgView = a.msgView.LineDown(4)
		return a, nil
	}

	if key.Matches(msg, Keys.FocusSidebar) {
		a.focus = FocusSidebarArea
		a.sidebar = a.sidebar.SetFocused(true)
		a.msgView = a.msgView.SetFocused(false)
		a.membersView = a.membersView.SetFocused(false)
		a.input = a.input.Blur()
		return a, nil
	}
	if key.Matches(msg, Keys.FocusChat) {
		a.focus = FocusChatArea
		a.sidebar = a.sidebar.SetFocused(false)
		a.msgView = a.msgView.SetFocused(true)
		a.membersView = a.membersView.SetFocused(false)
		a.input = a.input.Blur()
		return a, nil
	}
	if key.Matches(msg, Keys.FocusInput) {
		a.focus = FocusInputArea
		a.sidebar = a.sidebar.SetFocused(false)
		a.msgView = a.msgView.SetFocused(false)
		a.membersView = a.membersView.SetFocused(false)
		a.input = a.input.Focus()
		return a, nil
	}
	if key.Matches(msg, Keys.CreateGuild) {
		a.modal = a.modal.Show(components.ModalCreateGuild, "Create New Server")
		return a, nil
	}
	if key.Matches(msg, Keys.CreateChannel) && a.currentGuild != nil {
		a.modal = a.modal.Show(components.ModalCreateChannel, "Create New Channel")
		return a, nil
	}
	if key.Matches(msg, Keys.Help) && a.focus != FocusInputArea {
		a.modal = a.modal.Show(components.ModalHelp, "Keyboard Shortcuts")
		return a, nil
	}

	switch a.focus {
	case FocusSidebarArea:
		var cmd tea.Cmd
		a.sidebar, cmd = a.sidebar.Update(msg)
		cmds = append(cmds, cmd)
	case FocusChatArea:
		switch msg.String() {
		case "up", "k":
			a.msgView = a.msgView.LineUp(1)
			return a, nil
		case "down", "j":
			a.msgView = a.msgView.LineDown(1)
			return a, nil
		case "home", "g":
			a.msgView = a.msgView.GotoTop()
			return a, nil
		case "end", "G":
			a.msgView = a.msgView.GotoBottom()
			return a, nil
		case "pgup", "ctrl+u":
			a.msgView = a.msgView.PageUp()
			return a, nil
		case "pgdown", "ctrl+d":
			a.msgView = a.msgView.PageDown()
			return a, nil
		case "enter", "i":
			a.focus = FocusInputArea
			a.msgView = a.msgView.SetFocused(false).GotoBottom()
			a.input = a.input.Focus()
			return a, nil
		default:
			var cmd tea.Cmd
			a.msgView, cmd = a.msgView.Update(msg)
			cmds = append(cmds, cmd)
		}
	case FocusInputArea:
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		cmds = append(cmds, cmd)
	case FocusRightSidebar:
		var cmd tea.Cmd
		a.membersView, cmd = a.membersView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

func (a *App) View() string {
	switch a.viewState {
	case ViewLogin:
		return a.renderLoginView()
	case ViewChat:
		return a.renderChatView()
	default:
		return "Loading..."
	}
}

func (a *App) renderLoginView() string {
	h := a.height - 1
	if h < 10 {
		h = a.height
	}
	return lipgloss.Place(
		a.width, h,
		lipgloss.Center, lipgloss.Center,
		a.loginView.View(),
	)
}

// SessionID returns the unique session ID of this client.
func (a *App) SessionID() string {
	return a.sessionID
}

func (a *App) renderChatView() string {
	showRightSidebar := a.width >= 80

	leftSidebarWidth := 22
	if a.width < 90 {
		leftSidebarWidth = 18
	}

	rightSidebarWidth := 0
	if showRightSidebar {
		rightSidebarWidth = 20
		if a.width < 100 {
			rightSidebarWidth = 18
		}
	}

	chatWidth := a.width - leftSidebarWidth - rightSidebarWidth - 1
	if showRightSidebar {
		chatWidth = a.width - leftSidebarWidth - rightSidebarWidth - 2
	}
	if chatWidth < 10 {
		chatWidth = 10
	}

	contentHeight := a.height - 3
	if contentHeight < 5 {
		contentHeight = 5
	}
	inputHeight := 3
	msgHeight := contentHeight - inputHeight
	if msgHeight < 1 {
		msgHeight = 1
	}

	leftBorderColor := styles.SurfaceLight
	if a.focus == FocusSidebarArea {
		leftBorderColor = styles.Primary
	}

	leftSidebar := lipgloss.NewStyle().
		Width(leftSidebarWidth).
		Height(contentHeight).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(leftBorderColor).
		Render(a.sidebar.View())

	messages := lipgloss.NewStyle().
		Width(chatWidth).
		Height(msgHeight).
		Render(a.msgView.View())

	inputBar := lipgloss.NewStyle().
		Width(chatWidth).
		Render(a.input.View())

	centerPanel := lipgloss.JoinVertical(lipgloss.Left, messages, inputBar)

	var main string
	if showRightSidebar {
		rightBorderColor := styles.SurfaceLight
		if a.focus == FocusRightSidebar {
			rightBorderColor = styles.Primary
		}

		rightSidebar := lipgloss.NewStyle().
			Width(rightSidebarWidth).
			Height(contentHeight).
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(rightBorderColor).
			Render(a.membersView.View())

		main = lipgloss.JoinHorizontal(lipgloss.Top, leftSidebar, centerPanel, rightSidebar)
	} else {
		main = lipgloss.JoinHorizontal(lipgloss.Top, leftSidebar, centerPanel)
	}

	statusBar := a.statusBar.SetSize(a.width).View()
	result := lipgloss.JoinVertical(lipgloss.Left, main, "", statusBar)

	if a.modal.IsVisible() {
		h := a.height - 1
		if h < 10 {
			h = a.height
		}
		result = lipgloss.Place(
			a.width, h,
			lipgloss.Center, lipgloss.Center,
			a.modal.View(),
		)
	}

	return result
}

func (a *App) resizeAll() {
	showRightSidebar := a.width >= 80

	leftSidebarWidth := 22
	if a.width < 90 {
		leftSidebarWidth = 18
	}

	rightSidebarWidth := 0
	if showRightSidebar {
		rightSidebarWidth = 20
		if a.width < 100 {
			rightSidebarWidth = 18
		}
	}

	chatWidth := a.width - leftSidebarWidth - rightSidebarWidth - 1
	if showRightSidebar {
		chatWidth = a.width - leftSidebarWidth - rightSidebarWidth - 2
	}
	if chatWidth < 10 {
		chatWidth = 10
	}
	contentHeight := a.height - 3
	if contentHeight < 5 {
		contentHeight = 5
	}
	msgHeight := contentHeight - 3
	if msgHeight < 1 {
		msgHeight = 1
	}
	a.loginView = a.loginView.SetSize(a.width, a.height)
	a.sidebar = a.sidebar.SetSize(leftSidebarWidth, contentHeight)
	a.msgView = a.msgView.SetSize(chatWidth, msgHeight)
	a.input = a.input.SetSize(chatWidth)
	if showRightSidebar {
		a.membersView = a.membersView.SetSize(rightSidebarWidth, contentHeight)
	}
	a.statusBar = a.statusBar.SetSize(a.width)
}

func (a *App) waitForActorMsg() tea.Cmd {
	if a.sessionActor == nil {
		return nil
	}
	ch := a.sessionActor.MsgChan()
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (a *App) loadGuilds() tea.Cmd {
	return func() tea.Msg {
		if a.user == nil {
			return nil
		}
		ctx := context.Background()

		// Always enroll user into the shared Shell Chat Community guild
		_, _ = a.guildStore.EnsureDefaultCommunityGuild(ctx, a.user.ID)

		guilds, err := a.guildStore.GetUserGuilds(ctx, a.user.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}

		return GuildsLoadedMsg{Guilds: guilds}
	}
}

func (a *App) loadChannels(guildID int64) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		channels, err := a.coalescer.GetGuildChannels(ctx, guildID)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return ChannelsLoadedMsg{Channels: channels}
	}
}

func (a *App) loadMessages(channelID int64) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		messages, err := a.coalescer.GetMessages(ctx, channelID, 0, 50)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return MessagesLoadedMsg{Messages: messages}
	}
}

func (a *App) selectChannel(ch *models.Channel) tea.Cmd {
	if ch == nil {
		return nil
	}

	// 1. Leave previous channel actor and unsubscribe from topic
	if a.currentChannel != nil && a.currentChannel.ID != ch.ID && a.user != nil {
		prevActorID := fmt.Sprintf("channel:%d", a.currentChannel.ID)
		if ref, ok := a.registry.Get(prevActorID); ok {
			ref.Send(actor.LeaveChannel{
				UserID:    a.user.ID,
				SessionID: a.sessionID,
			})
		}
		if a.subscriber != nil && a.currentGuild != nil {
			a.subscriber.UnsubscribeChannel(a.currentGuild.ID, a.currentChannel.ID)
		}
	}

	// 2. Set new active channel
	a.currentChannel = ch
	a.currentDMUserID = 0
	a.sidebar = a.sidebar.SetActiveChannel(ch.ID).ClearUnread(ch.ID)
	a.membersView = a.membersView.SetActiveDM(0)
	a.statusBar = a.statusBar.SetChannel(ch.Name)
	a.msgView = a.msgView.SetChannel(ch.Name, ch.Topic)
	if ch.Type == models.ChannelTypeAnnouncement {
		a.input = a.input.SetDisabled(true).SetPlaceholder("#announcements is read-only")
		a.focus = FocusChatArea
		a.msgView = a.msgView.SetFocused(true)
		a.sidebar = a.sidebar.SetFocused(false)
		a.membersView = a.membersView.SetFocused(false)
	} else {
		a.input = a.input.SetDisabled(false).SetPlaceholder(fmt.Sprintf("Message #%s", ch.Name)).Focus()
		a.focus = FocusInputArea
		a.msgView = a.msgView.SetFocused(false)
		a.sidebar = a.sidebar.SetFocused(false)
		a.membersView = a.membersView.SetFocused(false)
	}

	if a.sessionActor != nil {
		guildID := int64(0)
		if a.currentGuild != nil {
			guildID = a.currentGuild.ID
		}
		a.sessionActor.SetActiveContext(guildID, ch.ID)
	}

	// 3. Join new channel actor
	if a.user != nil {
		guildID := ch.GuildID
		if guildID == 0 && a.currentGuild != nil {
			guildID = a.currentGuild.ID
		}
		channelRef := a.registry.GetOrCreateChannelActor(ch.ID, guildID, a.msgStore, a.broker, a.logger)
		channelRef.Send(actor.JoinChannel{
			UserID:    a.user.ID,
			SessionID: a.sessionID,
		})

		// 4. Subscribe to Redis Pub/Sub topic
		if a.subscriber != nil && guildID != 0 {
			_ = a.subscriber.SubscribeChannel(context.Background(), guildID, ch.ID)
		}
	}

	return a.loadMessages(ch.ID)
}

func (a *App) sendExistingMessage(msg *models.Message) tea.Cmd {
	return func() tea.Msg {
		guildID := int64(0)
		if a.currentGuild != nil {
			guildID = a.currentGuild.ID
		}
		if a.currentChannel != nil && a.currentChannel.GuildID != 0 {
			guildID = a.currentChannel.GuildID
		}

		channelRef := a.registry.GetOrCreateChannelActor(msg.ChannelID, guildID, a.msgStore, a.broker, a.logger)
		channelRef.Send(actor.PostMessage{
			Msg:          msg,
			TargetUserID: a.currentDMUserID,
		})
		return nil
	}
}

func (a *App) sendMessagesSequential(msgs ...*models.Message) tea.Cmd {
	return func() tea.Msg {
		guildID := int64(0)
		if a.currentGuild != nil {
			guildID = a.currentGuild.ID
		}
		if a.currentChannel != nil && a.currentChannel.GuildID != 0 {
			guildID = a.currentChannel.GuildID
		}

		for i, msg := range msgs {
			if i > 0 {
				time.Sleep(30 * time.Millisecond) // Ensure deterministic delivery sequence
			}
			channelRef := a.registry.GetOrCreateChannelActor(msg.ChannelID, guildID, a.msgStore, a.broker, a.logger)
			channelRef.Send(actor.PostMessage{
				Msg:          msg,
				TargetUserID: a.currentDMUserID,
			})
		}
		return nil
	}
}

// SparkResponseMsg contains the AI answer from Spark Bot.
type SparkResponseMsg struct {
	Answer         string
	ChannelID      int64
	GuildID        int64
	TargetDMUserID int64
}

func (a *App) querySparkBot(prompt string, userID int64, username string, channelID, guildID, targetDMUserID int64) tea.Cmd {
	cfg := bot.AIConfig{
		GeminiAPIKey: a.geminiAPIKey,
		GeminiModel:  a.geminiModel,
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		answer, _ := bot.AskAI(ctx, cfg, channelID, userID, username, prompt)
		return SparkResponseMsg{
			Answer:         answer,
			ChannelID:      channelID,
			GuildID:        guildID,
			TargetDMUserID: targetDMUserID,
		}
	}
}

func (a *App) createGuild(name string) tea.Cmd {
	return func() tea.Msg {
		if a.user == nil {
			return nil
		}
		ctx := context.Background()
		guild := &models.Guild{
			ID:      sfgen.Generate(),
			Name:    name,
			OwnerID: a.user.ID,
		}
		if err := a.guildStore.CreateGuild(ctx, guild); err != nil {
			return ErrorMsg{Err: err}
		}
		member := &models.GuildMember{
			GuildID: guild.ID,
			UserID:  a.user.ID,
			Role:    "owner",
		}
		if err := a.guildStore.AddMember(ctx, member); err != nil {
			return ErrorMsg{Err: err}
		}
		ch := &models.Channel{
			ID:      sfgen.Generate(),
			GuildID: guild.ID,
			Name:    "general",
			Topic:   "Welcome to " + name + "!",
			Type:    models.ChannelTypeText,
		}
		if err := a.guildStore.CreateChannel(ctx, ch); err != nil {
			return ErrorMsg{Err: err}
		}
		guilds, _ := a.coalescer.GetUserGuilds(ctx, a.user.ID)
		return GuildsLoadedMsg{Guilds: guilds}
	}
}

func (a *App) createChannel(name string) tea.Cmd {
	return func() tea.Msg {
		if a.user == nil || a.currentGuild == nil {
			return nil
		}
		ctx := context.Background()
		ch := &models.Channel{
			ID:      sfgen.Generate(),
			GuildID: a.currentGuild.ID,
			Name:    name,
			Type:    models.ChannelTypeText,
		}
		if err := a.guildStore.CreateChannel(ctx, ch); err != nil {
			return ErrorMsg{Err: err}
		}
		channels, _ := a.coalescer.GetGuildChannels(ctx, a.currentGuild.ID)
		return ChannelsLoadedMsg{Channels: channels}
	}
}

func (a *App) cleanup() {
	if a.subscriber != nil {
		a.subscriber.UnsubscribeAll()
	}
	if a.currentChannel != nil && a.user != nil {
		if ref, ok := a.registry.Get(fmt.Sprintf("channel:%d", a.currentChannel.ID)); ok {
			ref.Send(actor.LeaveChannel{
				UserID:    a.user.ID,
				SessionID: a.sessionID,
			})
		}
	}
	a.registry.Unregister("session:" + a.sessionID)
}
