package activitylog

import (
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func effectiveName(name string) discord.Member {
	return discord.Member{
		Nick: &name,
		User: discord.User{Username: "backup", GlobalName: &name},
	}
}

func effectiveNameGlobal(name string) discord.Member {
	return discord.Member{
		User: discord.User{Username: "backup", GlobalName: &name},
	}
}

func memberNoNick(username string) discord.Member {
	return discord.Member{
		Nick: nil,
		User: discord.User{Username: username},
	}
}

func TestJoinEmbed(t *testing.T) {
	member := effectiveName("Alice")
	embed := joinEmbed(member)

	if embed.Title != "**Alice** ist dem Server beigetreten" {
		t.Fatalf("expected title %q, got %q", "**Alice** ist dem Server beigetreten", embed.Title)
	}
	if embed.Color != 0x57F287 {
		t.Fatalf("expected color 0x57F287, got 0x%X", embed.Color)
	}
}

func TestLeaveEmbed(t *testing.T) {
	member := effectiveName("Bob")
	embed := leaveEmbed(member)

	if embed.Title != "**Bob** hat den Server verlassen" {
		t.Fatalf("expected leave title, got %q", embed.Title)
	}
	if embed.Color != 0xED4245 {
		t.Fatalf("expected color 0xED4245, got 0x%X", embed.Color)
	}
}

func TestNickChangeEmbed(t *testing.T) {
	member := effectiveName("NewName")
	embed := nickChangeEmbed(member, "OldName", "NewName")

	if embed.Title != "**NewName** hat den Nicknamen von **OldName** zu **NewName** geändert" {
		t.Fatalf("expected nick change title, got %q", embed.Title)
	}
	if embed.Color != 0x95A5A6 {
		t.Fatalf("expected color 0x95A5A6, got 0x%X", embed.Color)
	}
}

func TestRoleAddedEmbed(t *testing.T) {
	member := effectiveName("Charlie")
	embed := roleAddedEmbed(member, "Trucker")

	if embed.Title != "**Charlie** hat die Rolle **Trucker** erhalten" {
		t.Fatalf("expected role added title, got %q", embed.Title)
	}
	if embed.Color != 0x5865F2 {
		t.Fatalf("expected color 0x5865F2, got 0x%X", embed.Color)
	}
}

func TestRoleRemovedEmbed(t *testing.T) {
	member := effectiveName("Diana")
	embed := roleRemovedEmbed(member, "Filmschauer")

	if embed.Title != "**Diana** hat die Rolle **Filmschauer** verloren" {
		t.Fatalf("expected role removed title, got %q", embed.Title)
	}
	if embed.Color != 0xE67E22 {
		t.Fatalf("expected color 0xE67E22, got 0x%X", embed.Color)
	}
}

func TestVoiceJoinEmbed(t *testing.T) {
	member := effectiveName("Eve")
	embed := voiceJoinEmbed(member, "Allgemein")

	if embed.Title != "**Eve** hat den Voice-Channel **#Allgemein** betreten" {
		t.Fatalf("expected voice join title, got %q", embed.Title)
	}
	if embed.Color != 0x3498DB {
		t.Fatalf("expected color 0x3498DB, got 0x%X", embed.Color)
	}
}

func TestVoiceLeaveEmbed(t *testing.T) {
	member := effectiveName("Frank")
	embed := voiceLeaveEmbed(member, "Gaming")

	if embed.Title != "**Frank** hat den Voice-Channel **#Gaming** verlassen" {
		t.Fatalf("expected voice leave title, got %q", embed.Title)
	}
	if embed.Color != 0x3498DB {
		t.Fatalf("expected color 0x3498DB, got 0x%X", embed.Color)
	}
}

func TestVoiceMoveEmbed(t *testing.T) {
	member := effectiveName("Grace")
	embed := voiceMoveEmbed(member, "Allgemein", "Gaming")

	if embed.Title != "**Grace** ist von **#Allgemein** nach **#Gaming** gewechselt" {
		t.Fatalf("expected voice move title, got %q", embed.Title)
	}
	if embed.Color != 0x3498DB {
		t.Fatalf("expected color 0x3498DB, got 0x%X", embed.Color)
	}
}

func TestNickDiff(t *testing.T) {
	old := effectiveName("OldNick")
	new := effectiveName("NewNick")

	oldName, newName, changed := nickDiff(old, new)
	if !changed {
		t.Fatalf("expected changed=true when nicks differ")
	}
	if oldName != "OldNick" || newName != "NewNick" {
		t.Fatalf("expected OldNick→NewNick, got %s→%s", oldName, newName)
	}

	same := effectiveName("Same")
	_, _, changed = nickDiff(same, same)
	if changed {
		t.Fatalf("expected changed=false when nicks are the same")
	}

	noNick := memberNoNick("username")
	withNick := effectiveName("WithNick")
	oldName, newName, changed = nickDiff(noNick, withNick)
	if !changed {
		t.Fatalf("expected changed=true when nick was added")
	}
	if oldName != "username" || newName != "WithNick" {
		t.Fatalf("expected username→WithNick, got %s→%s", oldName, newName)
	}

	oldGlobal := effectiveNameGlobal("OldGlobal")
	newGlobal := effectiveNameGlobal("NewGlobal")
	oldName, newName, changed = nickDiff(oldGlobal, newGlobal)
	if !changed {
		t.Fatalf("expected changed=true when global names differ")
	}
	if oldName != "OldGlobal" || newName != "NewGlobal" {
		t.Fatalf("expected OldGlobal→NewGlobal, got %s→%s", oldName, newName)
	}
}

func TestRoleDiff(t *testing.T) {
	roleA := snowflake.ID(1)
	roleB := snowflake.ID(2)
	roleC := snowflake.ID(3)

	added, removed := roleDiff([]snowflake.ID{roleA, roleB}, []snowflake.ID{roleA, roleB})
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("expected no changes, got added=%v removed=%v", added, removed)
	}

	added, removed = roleDiff([]snowflake.ID{roleA}, []snowflake.ID{roleA, roleB})
	if len(added) != 1 || len(removed) != 0 {
		t.Fatalf("expected 1 added, got added=%v removed=%v", added, removed)
	}
	if added[0] != roleB {
		t.Fatalf("expected roleB added, got %s", added[0])
	}

	added, removed = roleDiff([]snowflake.ID{roleA, roleB}, []snowflake.ID{roleA})
	if len(added) != 0 || len(removed) != 1 {
		t.Fatalf("expected 1 removed, got added=%v removed=%v", added, removed)
	}
	if removed[0] != roleB {
		t.Fatalf("expected roleB removed, got %s", removed[0])
	}

	added, removed = roleDiff([]snowflake.ID{roleA, roleB}, []snowflake.ID{roleA, roleC})
	if len(added) != 1 || len(removed) != 1 {
		t.Fatalf("expected 1 added + 1 removed, got added=%v removed=%v", added, removed)
	}
	if added[0] != roleC || removed[0] != roleB {
		t.Fatalf("expected C added, B removed, got added=%v removed=%v", added, removed)
	}

	added, removed = roleDiff(nil, []snowflake.ID{roleA})
	if len(added) != 1 || len(removed) != 0 {
		t.Fatalf("expected 1 added from nil, got added=%v removed=%v", added, removed)
	}

	added, removed = roleDiff([]snowflake.ID{roleA}, nil)
	if len(added) != 0 || len(removed) != 1 {
		t.Fatalf("expected 1 removed, got added=%v removed=%v", added, removed)
	}
}

func TestMemberName(t *testing.T) {
	member := discord.Member{
		Nick: ptr("Nickname"),
		User: discord.User{Username: "Username", GlobalName: ptr("Global")},
	}
	if memberName(member) != "Nickname" {
		t.Fatalf("expected Nickname, got %q", memberName(member))
	}

	member = discord.Member{
		Nick: nil,
		User: discord.User{Username: "Username", GlobalName: ptr("Global")},
	}
	if memberName(member) != "Global" {
		t.Fatalf("expected Global, got %q", memberName(member))
	}

	member = discord.Member{
		Nick: nil,
		User: discord.User{Username: "Username"},
	}
	if memberName(member) != "Username" {
		t.Fatalf("expected Username, got %q", memberName(member))
	}
}

func ptr[T any](v T) *T { return &v }

func TestIsRecentDuplicate(t *testing.T) {
	l := New(0)

	userID := snowflake.MustParse("111111111111111111")

	if l.isRecentDuplicate(userID, "join") {
		t.Fatal("first event should not be duplicate")
	}

	if !l.isRecentDuplicate(userID, "join") {
		t.Fatal("second event within 3s should be duplicate")
	}

	if l.isRecentDuplicate(userID, "leave") {
		t.Fatal("different event type should not be duplicate")
	}
}

func TestEventCooldownExpires(t *testing.T) {
	l := New(0)
	userID := snowflake.MustParse("111111111111111111")

	l.isRecentDuplicate(userID, "join")

	l.eventMu.Lock()
	k := eventKey{userID: userID, eventType: "join"}
	l.eventCooldown[k] = time.Now().Add(-4 * time.Second)
	l.eventMu.Unlock()

	if l.isRecentDuplicate(userID, "join") {
		t.Fatal("event after cooldown should not be duplicate")
	}
}

func TestIsRealVoiceMove(t *testing.T) {
	chA := ptr(snowflake.MustParse("111111111111111111"))
	chB := ptr(snowflake.MustParse("222222222222222222"))

	if isRealVoiceMove(chA, chB) != true {
		t.Fatal("different channels should be a real move")
	}
	if isRealVoiceMove(chA, chA) {
		t.Fatal("same channel is not a real move")
	}
	if isRealVoiceMove(nil, chB) {
		t.Fatal("nil from is not a move")
	}
	if isRealVoiceMove(chA, nil) {
		t.Fatal("nil to is not a move")
	}
}

func TestEmbedHasTimestampFooter(t *testing.T) {
	member := discord.Member{
		User: discord.User{
			ID:         snowflake.MustParse("111111111111111111"),
			Username:   "TestUser",
			GlobalName: &[]string{"TestUser"}[0],
		},
	}

	embeds := []discord.Embed{
		joinEmbed(member),
		leaveEmbed(member),
		nickChangeEmbed(member, "old", "new"),
		roleAddedEmbed(member, "Admin"),
		roleRemovedEmbed(member, "Admin"),
		voiceJoinEmbed(member, "General"),
		voiceLeaveEmbed(member, "General"),
		voiceMoveEmbed(member, "A", "B"),
	}

	for i, embed := range embeds {
		if embed.Footer == nil {
			t.Fatalf("embed %d: expected Footer, got nil", i)
		}
		if embed.Footer.Text == "" {
			t.Fatalf("embed %d: expected non-empty Footer text", i)
		}
		_, err := time.Parse("02.01.2006 15:04:05", embed.Footer.Text)
		if err != nil {
			t.Fatalf("embed %d: Footer text %q is not valid timestamp: %v", i, embed.Footer.Text, err)
		}
	}
}
