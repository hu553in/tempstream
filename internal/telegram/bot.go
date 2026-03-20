package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"tempstream/internal/service"
)

const (
	commandWhoAmI  = "/whoami"
	commandOff     = "/off"
	commandOffLast = "/offlast"
	commandNew     = "/new"
	commandNewPerm = "/newperm"
	buttonMenu     = "Menu"
	linkTimeFormat = "02.01.2006 15:04"
	menuStaticRows = 3
)

type Bot struct {
	raw            *tgbot.Bot
	username       string
	log            *slog.Logger
	allowedChatIDs []int64
	links          *service.LinkService
	location       *time.Location
	ttlOptions     []time.Duration
}

func New(
	token string,
	allowedChatIDs []int64,
	links *service.LinkService,
	ttlOptions []time.Duration,
	location *time.Location,
	log *slog.Logger,
) (*Bot, error) {
	if location == nil {
		location = time.UTC
	}

	b := &Bot{
		log:            log,
		allowedChatIDs: allowedChatIDs,
		links:          links,
		location:       location,
		ttlOptions:     append([]time.Duration(nil), ttlOptions...),
	}

	raw, err := tgbot.New(token, tgbot.WithDefaultHandler(b.handleMessage))
	if err != nil {
		return nil, err
	}

	b.raw = raw

	me, err := raw.GetMe(context.Background())
	if err != nil {
		return nil, err
	}
	b.username = strings.TrimSpace(me.Username)

	return b, nil
}

func (b *Bot) Start(ctx context.Context) {
	b.log.InfoContext(ctx, "telegram bot started")
	b.raw.Start(ctx)
}

func (b *Bot) handleMessage(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := normalizeMessageText(update.Message.Text)

	if text == commandWhoAmI {
		b.reply(ctx, chatID, fmt.Sprintf("Your chat_id: %d", chatID), nil)
		return
	}

	if !slices.Contains(b.allowedChatIDs, chatID) {
		b.reply(ctx, chatID, "Access denied.", nil)
		return
	}

	if text == "" {
		return
	}

	switch {
	case strings.HasPrefix(text, "/start "):
		if b.handleStartPayload(ctx, chatID, strings.TrimSpace(strings.TrimPrefix(text, "/start"))) {
			return
		}
		b.sendMenu(ctx, chatID)

	case text == "/start" || text == "/help" || text == buttonMenu:
		b.sendMenu(ctx, chatID)

	case strings.HasPrefix(text, commandNew+" "):
		ttl, ok := b.parseTTLCommand(text)
		if !ok {
			b.reply(ctx, chatID, b.newUsageText(), menuKeyboard(b.ttlOptions))
			return
		}
		b.createTTLLink(ctx, chatID, ttl)

	case text == commandNewPerm || text == "♾ Permanent":
		b.createPermanentLink(ctx, chatID)

	case text == "/active" || text == "📋 Active":
		b.listActive(ctx, chatID)

	case text == "/status" || text == "📡 Status":
		b.status(ctx, chatID)

	case text == commandOffLast || text == "⛔ Disable last":
		b.offLast(ctx, chatID)

	case b.matchTTLButton(text):
		ttl, ok := b.parseTTLButton(text)
		if !ok {
			b.reply(ctx, chatID, b.newUsageText(), menuKeyboard(b.ttlOptions))
			return
		}
		b.createTTLLink(ctx, chatID, ttl)

	case text == commandOff:
		b.reply(ctx, chatID, "Usage: /off 123", menuKeyboard(b.ttlOptions))

	case strings.HasPrefix(text, commandOff+" "):
		idStr := strings.TrimSpace(strings.TrimPrefix(text, commandOff))
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			b.reply(ctx, chatID, "Usage: /off 123", menuKeyboard(b.ttlOptions))
			return
		}
		b.offByID(ctx, chatID, id)

	default:
		b.reply(ctx, chatID, "Unknown command. Open \"Menu\" to continue.", menuKeyboard(b.ttlOptions))
	}
}

func (b *Bot) createTTLLink(ctx context.Context, chatID int64, ttl time.Duration) {
	link, url, err := b.links.CreateLink(ctx, ttl, "")
	if err != nil {
		b.log.ErrorContext(ctx, "create watch link failed",
			slog.Int64("chat_id", chatID),
			slog.Duration("ttl", ttl),
			slog.String("err", err.Error()),
		)
		b.reply(ctx, chatID, "Failed to create link.", menuKeyboard(b.ttlOptions))
		return
	}

	exp := "never"
	if link.ExpiresAt != nil {
		exp = b.formatTime(*link.ExpiresAt)
	}

	msg := fmt.Sprintf(
		"🟢 Link created\n\nID: %d\nExpires: %s\nURL: %s\nDisable: %s",
		link.ID, exp, url, b.disableLinkAction(link.ID),
	)
	b.replyHTML(ctx, chatID, msg, menuKeyboard(b.ttlOptions))
}

func (b *Bot) createPermanentLink(ctx context.Context, chatID int64) {
	link, url, err := b.links.CreatePermanentLink(ctx, "")
	if err != nil {
		b.log.ErrorContext(ctx, "create permanent watch link failed",
			slog.Int64("chat_id", chatID),
			slog.String("err", err.Error()),
		)
		b.reply(ctx, chatID, "Failed to create link.", menuKeyboard(b.ttlOptions))
		return
	}

	msg := fmt.Sprintf(
		"🟢 Permanent link created\n\nID: %d\nExpires: never\nURL: %s\nDisable: %s",
		link.ID, url, b.disableLinkAction(link.ID),
	)
	b.replyHTML(ctx, chatID, msg, menuKeyboard(b.ttlOptions))
}

func (b *Bot) listActive(ctx context.Context, chatID int64) {
	links, err := b.links.ListActive(ctx)
	if err != nil {
		b.log.ErrorContext(ctx, "list active watch links failed",
			slog.Int64("chat_id", chatID),
			slog.String("err", err.Error()),
		)
		b.reply(ctx, chatID, "Failed to get active links.", menuKeyboard(b.ttlOptions))
		return
	}

	if len(links) == 0 {
		b.reply(ctx, chatID, "No active links.", menuKeyboard(b.ttlOptions))
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 Active links:\n\n")

	for _, link := range links {
		_, _ = fmt.Fprintf(&sb, "ID: %d\n", link.ID)
		_, _ = fmt.Fprintf(&sb, "URL: %s\n", b.links.WatchURL(link.Token))
		if link.ExpiresAt != nil {
			sb.WriteString("Expires: " + b.formatTime(*link.ExpiresAt) + "\n")
		} else {
			sb.WriteString("Expires: never\n")
		}
		sb.WriteString("Disable: " + b.disableLinkAction(link.ID) + "\n")
		sb.WriteString("\n")
	}

	b.replyHTML(ctx, chatID, sb.String(), menuKeyboard(b.ttlOptions))
}

func (b *Bot) status(ctx context.Context, chatID int64) {
	links, err := b.links.ListActive(ctx)
	if err != nil {
		b.log.ErrorContext(ctx, "get watch status failed",
			slog.Int64("chat_id", chatID),
			slog.String("err", err.Error()),
		)
		b.reply(ctx, chatID, "Failed to get status.", menuKeyboard(b.ttlOptions))
		return
	}

	streamStatus := "🔴 OFFLINE"
	if b.links.StreamLooksAlive(ctx) {
		streamStatus = "🟢 ONLINE"
	}

	msg := fmt.Sprintf(
		"📡 Stream status: %s\nActive links: %d",
		streamStatus,
		len(links),
	)
	b.reply(ctx, chatID, msg, menuKeyboard(b.ttlOptions))
}

func (b *Bot) offLast(ctx context.Context, chatID int64) {
	link, err := b.links.DisableLast(ctx)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			b.reply(ctx, chatID, "No active links.", menuKeyboard(b.ttlOptions))
			return
		}
		b.log.ErrorContext(ctx, "disable last watch link failed",
			slog.Int64("chat_id", chatID),
			slog.String("err", err.Error()),
		)
		b.reply(ctx, chatID, "Failed to disable link.", menuKeyboard(b.ttlOptions))
		return
	}

	msg := fmt.Sprintf(
		"⛔ Link disabled\n\nID: %d\nURL: %s",
		link.ID,
		b.links.WatchURL(link.Token),
	)
	b.reply(ctx, chatID, msg, menuKeyboard(b.ttlOptions))
}

func (b *Bot) offByID(ctx context.Context, chatID int64, id int64) {
	link, err := b.links.DisableByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			b.reply(ctx, chatID, "Link not found.", menuKeyboard(b.ttlOptions))
			return
		}
		b.log.ErrorContext(ctx, "disable watch link by id failed",
			slog.Int64("chat_id", chatID),
			slog.Int64("link_id", id),
			slog.String("err", err.Error()),
		)
		b.reply(ctx, chatID, "Failed to disable link.", menuKeyboard(b.ttlOptions))
		return
	}

	msg := fmt.Sprintf(
		"⛔ Link disabled\n\nID: %d\nURL: %s",
		link.ID,
		b.links.WatchURL(link.Token),
	)
	b.reply(ctx, chatID, msg, menuKeyboard(b.ttlOptions))
}

func (b *Bot) sendMenu(ctx context.Context, chatID int64) {
	b.reply(ctx, chatID, b.menuText(), menuKeyboard(b.ttlOptions))
}

func (b *Bot) reply(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	b.replyWithMode(ctx, chatID, text, markup, "")
}

func (b *Bot) replyHTML(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) {
	b.replyWithMode(ctx, chatID, text, markup, models.ParseModeHTML)
}

func (b *Bot) replyWithMode(
	ctx context.Context,
	chatID int64,
	text string,
	markup models.ReplyMarkup,
	parseMode models.ParseMode,
) {
	_, err := b.raw.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: markup,
		ParseMode:   parseMode,
	})
	if err != nil {
		b.log.ErrorContext(ctx, "telegram send failed",
			slog.Int64("chat_id", chatID),
			slog.String("err", err.Error()),
		)
	}
}

func (b *Bot) handleStartPayload(ctx context.Context, chatID int64, payload string) bool {
	id, ok := parseDisablePayload(payload)
	if !ok {
		return false
	}

	b.offByID(ctx, chatID, id)
	return true
}

func parseDisablePayload(payload string) (int64, bool) {
	if !strings.HasPrefix(payload, "disable_") {
		return 0, false
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(payload, "disable_"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}

	return id, true
}

func (b *Bot) disableLinkAction(id int64) string {
	if b.username == "" {
		return html.EscapeString(fmt.Sprintf("/off %d", id))
	}

	deepLink := fmt.Sprintf("https://t.me/%s?start=disable_%d", b.username, id)
	return fmt.Sprintf(`<a href="%s">Disable</a>`, html.EscapeString(deepLink))
}

func (b *Bot) formatTime(value time.Time) string {
	return value.In(b.location).Format(linkTimeFormat)
}

func (b *Bot) menuText() string {
	var sb strings.Builder
	sb.WriteString("👋 Watch bot\n\n")

	for _, ttl := range b.ttlOptions {
		_, _ = fmt.Fprintf(&sb, "/new %s - create a %s link\n", ttl.String(), ttlLabel(ttl))
	}

	sb.WriteString("/newperm - create a permanent link\n")
	sb.WriteString("/active - show active links\n")
	sb.WriteString("/status - show stream status\n")
	sb.WriteString("/offlast - disable the last link\n")
	sb.WriteString("/off ID - disable a link by ID\n")
	sb.WriteString("/whoami - show your chat ID")

	return sb.String()
}

func (b *Bot) newUsageText() string {
	if len(b.ttlOptions) == 0 {
		return "Temporary links are disabled. Use /newperm instead."
	}

	values := make([]string, 0, len(b.ttlOptions))
	for _, ttl := range b.ttlOptions {
		values = append(values, ttl.String())
	}

	return "Usage: /new <duration>\nAllowed values: " + strings.Join(values, ", ")
}

func (b *Bot) parseTTLCommand(text string) (time.Duration, bool) {
	value := strings.TrimSpace(strings.TrimPrefix(text, commandNew))
	ttl, err := time.ParseDuration(value)
	if err != nil || ttl <= 0 {
		return 0, false
	}

	if slices.Contains(b.ttlOptions, ttl) {
		return ttl, true
	}

	return 0, false
}

func (b *Bot) matchTTLButton(text string) bool {
	for _, ttl := range b.ttlOptions {
		if text == ttlButtonLabel(ttl) {
			return true
		}
	}

	return false
}

func (b *Bot) parseTTLButton(text string) (time.Duration, bool) {
	for _, ttl := range b.ttlOptions {
		if text == ttlButtonLabel(ttl) {
			return ttl, true
		}
	}

	return 0, false
}

func normalizeMessageText(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return text
	}

	command, rest, hasRest := strings.Cut(text, " ")
	command, _, _ = strings.Cut(command, "@")
	if !hasRest {
		return command
	}

	return command + " " + strings.TrimSpace(rest)
}

func menuKeyboard(ttlOptions []time.Duration) models.ReplyMarkup {
	keyboard := make([][]models.KeyboardButton, 0, len(ttlOptions)/2+menuStaticRows)

	for i := 0; i < len(ttlOptions); i += 2 {
		row := []models.KeyboardButton{{Text: ttlButtonLabel(ttlOptions[i])}}
		if i+1 < len(ttlOptions) {
			row = append(row, models.KeyboardButton{Text: ttlButtonLabel(ttlOptions[i+1])})
		}
		keyboard = append(keyboard, row)
	}

	keyboard = append(keyboard,
		[]models.KeyboardButton{{Text: "♾ Permanent"}},
		[]models.KeyboardButton{{Text: "📋 Active"}, {Text: "📡 Status"}},
		[]models.KeyboardButton{{Text: "⛔ Disable last"}, {Text: "Menu"}},
	)

	return &models.ReplyKeyboardMarkup{
		ResizeKeyboard: true,
		Keyboard:       keyboard,
	}
}

func ttlButtonLabel(ttl time.Duration) string {
	return "🟢 " + ttlLabel(ttl)
}

func ttlLabel(ttl time.Duration) string {
	switch {
	case ttl%time.Hour == 0:
		hours := int64(ttl / time.Hour)
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	case ttl%time.Minute == 0:
		minutes := int64(ttl / time.Minute)
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	default:
		return ttl.String()
	}
}
