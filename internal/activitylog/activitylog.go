package activitylog

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type eventKey struct {
	userID     snowflake.ID
	eventType  string
}

type ActivityLog struct {
	channelID     snowflake.ID
	lastSeqMu     sync.Mutex
	lastSeq       int
	hasLastSeq    bool
	memberRoles   map[snowflake.ID][]snowflake.ID
	memberRolesMu sync.RWMutex
	eventCooldown map[eventKey]time.Time
	eventMu       sync.Mutex
}

func New(channelID snowflake.ID) *ActivityLog {
	return &ActivityLog{
		channelID:     channelID,
		memberRoles:   make(map[snowflake.ID][]snowflake.ID),
		eventCooldown: make(map[eventKey]time.Time),
	}
}

func (l *ActivityLog) isRecentDuplicate(userID snowflake.ID, eventType string) bool {
	l.eventMu.Lock()
	defer l.eventMu.Unlock()
	key := eventKey{userID: userID, eventType: eventType}
	if last, ok := l.eventCooldown[key]; ok && time.Since(last) < 3*time.Second {
		return true
	}
	l.eventCooldown[key] = time.Now()
	return false
}

// ---- Embed builders ----

func joinEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** ist dem Server beigetreten", memberName(member)),
		Color:  0x57F287,
		Footer: timestampFooter(),
	}
}

func leaveEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** hat den Server verlassen", memberName(member)),
		Color:  0xED4245,
		Footer: timestampFooter(),
	}
}

func nickChangeEmbed(member discord.Member, oldName, newName string) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** hat den Nicknamen von **%s** zu **%s** geändert", memberName(member), oldName, newName),
		Color:  0x95A5A6,
		Footer: timestampFooter(),
	}
}

func roleAddedEmbed(member discord.Member, roleName string) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** hat die Rolle **%s** erhalten", memberName(member), roleName),
		Color:  0x5865F2,
		Footer: timestampFooter(),
	}
}

func roleRemovedEmbed(member discord.Member, roleName string) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** hat die Rolle **%s** verloren", memberName(member), roleName),
		Color:  0xE67E22,
		Footer: timestampFooter(),
	}
}

func voiceJoinEmbed(member discord.Member, channelName string) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** hat den Voice-Channel **#%s** betreten", memberName(member), channelName),
		Color:  0x3498DB,
		Footer: timestampFooter(),
	}
}

func voiceLeaveEmbed(member discord.Member, channelName string) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** hat den Voice-Channel **#%s** verlassen", memberName(member), channelName),
		Color:  0x3498DB,
		Footer: timestampFooter(),
	}
}

func voiceMoveEmbed(member discord.Member, fromChannel, toChannel string) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** ist von **#%s** nach **#%s** gewechselt", memberName(member), fromChannel, toChannel),
		Color:  0x3498DB,
		Footer: timestampFooter(),
	}
}

// ---- Helpers ----

func memberName(member discord.Member) string {
	return member.EffectiveName()
}

func timestampFooter() *discord.EmbedFooter {
	return &discord.EmbedFooter{Text: berlinTime().Format("02.01.2006 15:04:05")}
}

func berlinTime() time.Time {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

func nickDiff(oldMember, newMember discord.Member) (oldName, newName string, changed bool) {
	oldName = oldMember.EffectiveName()
	newName = newMember.EffectiveName()
	if oldName == "" {
		return oldName, newName, false
	}
	return oldName, newName, oldName != newName
}

func roleDiff(oldIDs, newIDs []snowflake.ID) (added, removed []snowflake.ID) {
	old := make(map[snowflake.ID]struct{}, len(oldIDs))
	for _, id := range oldIDs {
		old[id] = struct{}{}
	}
	nu := make(map[snowflake.ID]struct{}, len(newIDs))
	for _, id := range newIDs {
		nu[id] = struct{}{}
	}

	for _, id := range newIDs {
		if _, ok := old[id]; !ok {
			added = append(added, id)
		}
	}
	for _, id := range oldIDs {
		if _, ok := nu[id]; !ok {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func (l *ActivityLog) roleNames(client *bot.Client, guildID snowflake.ID) map[snowflake.ID]string {
	names := make(map[snowflake.ID]string)
	roles, err := client.Rest.GetRoles(guildID)
	if err != nil {
		slog.Warn("failed to fetch guild roles", slog.Any("err", err))
		return names
	}
	for _, role := range roles {
		names[role.ID] = role.Name
	}
	return names
}

func (l *ActivityLog) resolveRoleName(cacheNames map[snowflake.ID]string, client *bot.Client, guildID, roleID snowflake.ID) string {
	if name, ok := cacheNames[roleID]; ok {
		return name
	}
	return l.roleName(client, guildID, roleID)
}

func (l *ActivityLog) roleName(client *bot.Client, guildID, roleID snowflake.ID) string {
	if role, ok := client.Caches.Role(guildID, roleID); ok && role.Name != "" {
		return role.Name
	}
	roles, err := client.Rest.GetRoles(guildID)
	if err != nil {
		slog.Warn("failed to fetch guild roles", slog.Any("err", err))
		return roleID.String()
	}
	for _, role := range roles {
		if role.ID == roleID {
			return role.Name
		}
	}
	return roleID.String()
}

func (l *ActivityLog) channelName(client *bot.Client, channelID snowflake.ID) string {
	if channel, ok := client.Caches.GuildVoiceChannel(channelID); ok {
		return channel.Name()
	}
	channel, err := client.Rest.GetChannel(channelID)
	if err != nil {
		slog.Warn("failed to fetch channel", slog.Any("err", err))
		return channelID.String()
	}
	if gvc, ok := channel.(discord.GuildVoiceChannel); ok {
		return gvc.Name()
	}
	return channelID.String()
}

func (l *ActivityLog) post(client *bot.Client, embed discord.Embed, fields ...any) {
	if _, err := client.Rest.CreateMessage(l.channelID, discord.NewMessageCreate().WithEmbeds(embed)); err != nil {
		slog.Warn("failed to post activity log", append([]any{slog.Any("err", err)}, fields...)...)
		return
	}
	slog.Info("activity log posted", fields...)
}

func postFields(eventType string, member discord.Member, seq int) []any {
	return []any{
		slog.String("event", eventType),
		slog.String("user", memberName(member)),
		slog.String("user_id", member.User.ID.String()),
		slog.Int("seq", seq),
	}
}

func voicePostFields(eventType string, member discord.Member, channelID snowflake.ID, seq int) []any {
	return append(postFields(eventType, member, seq), slog.String("channel_id", channelID.String()))
}

// ---- Event handlers ----

func (l *ActivityLog) isDuplicate(seq int) bool {
	l.lastSeqMu.Lock()
	defer l.lastSeqMu.Unlock()
	if l.hasLastSeq && l.lastSeq == seq {
		return true
	}
	l.lastSeq = seq
	l.hasLastSeq = true
	return false
}

func (l *ActivityLog) OnGuildMemberJoin(event *events.GuildMemberJoin) {
	if l.isDuplicate(event.SequenceNumber()) {
		return
	}
	if l.isRecentDuplicate(event.Member.User.ID, "join") {
		return
	}
	l.memberRolesMu.Lock()
	l.memberRoles[event.Member.User.ID] = copyRoleIDs(event.Member.RoleIDs)
	l.memberRolesMu.Unlock()
	l.post(event.Client(), joinEmbed(event.Member), postFields("member_join", event.Member, event.SequenceNumber())...)
}

func (l *ActivityLog) OnGuildMemberLeave(event *events.GuildMemberLeave) {
	if l.isDuplicate(event.SequenceNumber()) {
		return
	}
	if l.isRecentDuplicate(event.User.ID, "leave") {
		return
	}
	l.memberRolesMu.Lock()
	delete(l.memberRoles, event.User.ID)
	l.memberRolesMu.Unlock()
	l.post(event.Client(), leaveEmbed(event.Member), postFields("member_leave", event.Member, event.SequenceNumber())...)
}

func (l *ActivityLog) OnGuildMemberUpdate(event *events.GuildMemberUpdate) {
	if l.isDuplicate(event.SequenceNumber()) {
		return
	}
	oldName, newName, changed := nickDiff(event.OldMember, event.Member)
	if changed {
		l.post(event.Client(), nickChangeEmbed(event.Member, oldName, newName), postFields("nick_change", event.Member, event.SequenceNumber())...)
	}

	l.memberRolesMu.Lock()
	oldRoles := l.memberRoles[event.Member.User.ID]
	l.memberRoles[event.Member.User.ID] = copyRoleIDs(event.Member.RoleIDs)
	l.memberRolesMu.Unlock()

	if oldRoles == nil {
		return
	}

	added, removed := roleDiff(oldRoles, event.Member.RoleIDs)
	names := l.roleNames(event.Client(), event.GuildID)
	for _, roleID := range added {
		l.post(event.Client(), roleAddedEmbed(event.Member, l.resolveRoleName(names, event.Client(), event.GuildID, roleID)), postFields("role_added", event.Member, event.SequenceNumber())...)
	}
	for _, roleID := range removed {
		l.post(event.Client(), roleRemovedEmbed(event.Member, l.resolveRoleName(names, event.Client(), event.GuildID, roleID)), postFields("role_removed", event.Member, event.SequenceNumber())...)
	}
}

func copyRoleIDs(ids []snowflake.ID) []snowflake.ID {
	out := make([]snowflake.ID, len(ids))
	copy(out, ids)
	return out
}

func (l *ActivityLog) OnGuildVoiceJoin(event *events.GuildVoiceJoin) {
	if l.isDuplicate(event.SequenceNumber()) || event.VoiceState.ChannelID == nil {
		return
	}
	if l.isRecentDuplicate(event.Member.User.ID, "voice_join") {
		return
	}
	channelID := *event.VoiceState.ChannelID
	l.post(event.Client(), voiceJoinEmbed(event.Member, l.channelName(event.Client(), channelID)), voicePostFields("voice_join", event.Member, channelID, event.SequenceNumber())...)
}

func (l *ActivityLog) OnGuildVoiceMove(event *events.GuildVoiceMove) {
	if l.isDuplicate(event.SequenceNumber()) {
		return
	}
	from := event.OldVoiceState.ChannelID
	to := event.VoiceState.ChannelID
	// disgo fires GuildVoiceMove for every voice state update where both
	// channels are set — including mute/stream toggles and state replays
	// while the member stays in the same channel. Those are not real moves.
	if !isRealVoiceMove(from, to) {
		return
	}
	if l.isRecentDuplicate(event.Member.User.ID, "voice_move") {
		return
	}
	l.post(event.Client(), voiceMoveEmbed(event.Member, l.channelName(event.Client(), *from), l.channelName(event.Client(), *to)), voicePostFields("voice_move", event.Member, *to, event.SequenceNumber())...)
}

func isRealVoiceMove(from, to *snowflake.ID) bool {
	return from != nil && to != nil && *from != *to
}

func (l *ActivityLog) OnGuildVoiceLeave(event *events.GuildVoiceLeave) {
	if l.isDuplicate(event.SequenceNumber()) {
		return
	}
	from := event.OldVoiceState.ChannelID
	if from == nil {
		return
	}
	if l.isRecentDuplicate(event.Member.User.ID, "voice_leave") {
		return
	}
	l.post(event.Client(), voiceLeaveEmbed(event.Member, l.channelName(event.Client(), *from)), voicePostFields("voice_leave", event.Member, *from, event.SequenceNumber())...)
}
