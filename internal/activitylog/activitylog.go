package activitylog

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type ActivityLog struct {
	channelID snowflake.ID
}

func New(channelID snowflake.ID) *ActivityLog {
	return &ActivityLog{channelID: channelID}
}

// ---- Embed builders ----

func joinEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** ist dem Server beigetreten", memberName(member)),
		Color: 0x57F287,
	}
}

func leaveEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat den Server verlassen", memberName(member)),
		Color: 0xED4245,
	}
}

func nickChangeEmbed(member discord.Member, oldName, newName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat den Nicknamen von **%s** zu **%s** geändert", memberName(member), oldName, newName),
		Color: 0x95A5A6,
	}
}

func roleAddedEmbed(member discord.Member, roleName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat die Rolle **%s** erhalten", memberName(member), roleName),
		Color: 0x5865F2,
	}
}

func roleRemovedEmbed(member discord.Member, roleName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat die Rolle **%s** verloren", memberName(member), roleName),
		Color: 0xE67E22,
	}
}

func voiceJoinEmbed(member discord.Member, channelName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat den Voice-Channel **#%s** betreten", memberName(member), channelName),
		Color: 0x3498DB,
	}
}

func voiceLeaveEmbed(member discord.Member, channelName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat den Voice-Channel **#%s** verlassen", memberName(member), channelName),
		Color: 0x3498DB,
	}
}

func voiceMoveEmbed(member discord.Member, fromChannel, toChannel string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** ist von **#%s** nach **#%s** gewechselt", memberName(member), fromChannel, toChannel),
		Color: 0x3498DB,
	}
}

// ---- Helpers ----

func memberName(member discord.Member) string {
	return member.EffectiveName()
}

func nickDiff(oldMember, newMember discord.Member) (oldName, newName string, changed bool) {
	oldName = oldMember.EffectiveName()
	newName = newMember.EffectiveName()
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

func (l *ActivityLog) roleName(client *bot.Client, guildID, roleID snowflake.ID) string {
	if role, ok := client.Caches.Role(guildID, roleID); ok && role.Name != "" {
		return role.Name
	}
	return roleID.String()
}

func (l *ActivityLog) channelName(client *bot.Client, channelID snowflake.ID) string {
	if channel, ok := client.Caches.GuildVoiceChannel(channelID); ok {
		return channel.Name()
	}
	return channelID.String()
}

func (l *ActivityLog) post(client *bot.Client, embed discord.Embed) {
	_, err := client.Rest.CreateMessage(l.channelID, discord.NewMessageCreate().WithEmbeds(embed))
	if err != nil {
		slog.Warn("failed to post activity log", slog.Any("err", err))
	}
}

// ---- Event handlers ----

func (l *ActivityLog) OnGuildMemberJoin(event *events.GuildMemberJoin) {
	l.post(event.Client(), joinEmbed(event.Member))
}

func (l *ActivityLog) OnGuildMemberLeave(event *events.GuildMemberLeave) {
	l.post(event.Client(), leaveEmbed(event.Member))
}

func (l *ActivityLog) OnGuildMemberUpdate(event *events.GuildMemberUpdate) {
	oldName, newName, changed := nickDiff(event.OldMember, event.Member)
	if changed {
		l.post(event.Client(), nickChangeEmbed(event.Member, oldName, newName))
	}

	added, removed := roleDiff(event.OldMember.RoleIDs, event.Member.RoleIDs)
	for _, roleID := range added {
		l.post(event.Client(), roleAddedEmbed(event.Member, l.roleName(event.Client(), event.GuildID, roleID)))
	}
	for _, roleID := range removed {
		l.post(event.Client(), roleRemovedEmbed(event.Member, l.roleName(event.Client(), event.GuildID, roleID)))
	}
}

func (l *ActivityLog) OnGuildVoiceJoin(event *events.GuildVoiceJoin) {
	if event.VoiceState.ChannelID == nil {
		return
	}
	l.post(event.Client(), voiceJoinEmbed(event.Member, l.channelName(event.Client(), *event.VoiceState.ChannelID)))
}

func (l *ActivityLog) OnGuildVoiceMove(event *events.GuildVoiceMove) {
	from := event.OldVoiceState.ChannelID
	to := event.VoiceState.ChannelID
	if from == nil || to == nil {
		return
	}
	l.post(event.Client(), voiceMoveEmbed(event.Member, l.channelName(event.Client(), *from), l.channelName(event.Client(), *to)))
}

func (l *ActivityLog) OnGuildVoiceLeave(event *events.GuildVoiceLeave) {
	from := event.OldVoiceState.ChannelID
	if from == nil {
		return
	}
	l.post(event.Client(), voiceLeaveEmbed(event.Member, l.channelName(event.Client(), *from)))
}
