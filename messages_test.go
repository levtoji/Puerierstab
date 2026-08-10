package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestMessageStoreLoadEmpty(t *testing.T) {
	// Clean up test file if it exists
	_ = os.Remove(".role_messages_test.json")
	defer os.Remove(".role_messages_test.json")

	// Test loading non-existent file
	store, err := loadMessageStore()
	if err != nil {
		t.Fatalf("loadMessageStore() error = %v, want nil", err)
	}
	if store == nil {
		t.Fatalf("expected store, got nil")
	}
	if len(store.Messages) != 0 {
		t.Fatalf("expected empty messages, got %d", len(store.Messages))
	}
}

func TestMessageStoreSetAndGet(t *testing.T) {
	store := &messageStore{Messages: []storedMessage{}}

	categoryName := "Gaming"
	messageIdx := 0
	messageID := snowflake.MustParse("123456789012345678")

	// Get should return false for non-existent
	_, exists := store.getMessageID(categoryName, messageIdx)
	if exists {
		t.Fatalf("expected exists=false for non-existent message")
	}

	// Set the message
	store.setMessageID(categoryName, messageIdx, messageID)

	// Get should now return true
	id, exists := store.getMessageID(categoryName, messageIdx)
	if !exists {
		t.Fatalf("expected exists=true after set")
	}
	if id != messageID {
		t.Fatalf("expected messageID %s, got %s", messageID, id)
	}
}

func TestMessageStoreUpdate(t *testing.T) {
	store := &messageStore{Messages: []storedMessage{}}

	categoryName := "Films"
	messageIdx := 0
	oldID := snowflake.MustParse("111111111111111111")
	newID := snowflake.MustParse("222222222222222222")

	// Set initial
	store.setMessageID(categoryName, messageIdx, oldID)

	// Update with new ID
	store.setMessageID(categoryName, messageIdx, newID)

	// Verify only one entry exists with new ID
	id, exists := store.getMessageID(categoryName, messageIdx)
	if !exists {
		t.Fatalf("expected exists=true after update")
	}
	if id != newID {
		t.Fatalf("expected updated messageID %s, got %s", newID, id)
	}
	if len(store.Messages) != 1 {
		t.Fatalf("expected 1 message stored, got %d", len(store.Messages))
	}
}

func TestMessageStoreMultipleEntries(t *testing.T) {
	store := &messageStore{Messages: []storedMessage{}}

	// Add entries for different categories and indices
	store.setMessageID("Gaming", 0, snowflake.MustParse("111111111111111111"))
	store.setMessageID("Gaming", 1, snowflake.MustParse("222222222222222222"))
	store.setMessageID("Films", 0, snowflake.MustParse("333333333333333333"))
	store.setMessageID("Films", 1, snowflake.MustParse("444444444444444444"))

	// Verify all entries
	tests := []struct {
		category string
		idx      int
		expected snowflake.ID
	}{
		{"Gaming", 0, snowflake.MustParse("111111111111111111")},
		{"Gaming", 1, snowflake.MustParse("222222222222222222")},
		{"Films", 0, snowflake.MustParse("333333333333333333")},
		{"Films", 1, snowflake.MustParse("444444444444444444")},
	}

	for _, tt := range tests {
		t.Run(tt.category+":"+string(rune(tt.idx)), func(t *testing.T) {
			id, exists := store.getMessageID(tt.category, tt.idx)
			if !exists {
				t.Fatalf("expected exists=true")
			}
			if id != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, id)
			}
		})
	}

	if len(store.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(store.Messages))
	}
}

func TestMessageStoreSaveAndLoad(t *testing.T) {
	testFile := ".role_messages_test.json"
	defer os.Remove(testFile)

	// Create and populate store
	store1 := &messageStore{
		Messages: []storedMessage{
			{
				CategoryName: "Gaming",
				MessageIndex: 0,
				MessageID:    snowflake.MustParse("111111111111111111"),
			},
			{
				CategoryName: "Films",
				MessageIndex: 0,
				MessageID:    snowflake.MustParse("222222222222222222"),
			},
		},
	}

	// Save to custom file (simulate)
	data, err := json.MarshalIndent(store1, "", "  ")
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	// Load from file
	fileData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var store2 messageStore
	if err := json.Unmarshal(fileData, &store2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify
	if len(store2.Messages) != 2 {
		t.Fatalf("expected 2 messages after load, got %d", len(store2.Messages))
	}
	if store2.Messages[0].CategoryName != "Gaming" {
		t.Fatalf("expected first message category Gaming, got %q", store2.Messages[0].CategoryName)
	}
}
