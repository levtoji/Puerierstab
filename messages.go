package main

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/disgoorg/snowflake/v2"
)

const messagesFile = ".role_messages.json"

type messageStore struct {
	Messages []storedMessage `json:"messages"`
	mu       sync.Mutex
}

type storedMessage struct {
	CategoryName  string `json:"category_name"`
	MessageIndex  int    `json:"message_index"`
	MessageID     snowflake.ID `json:"message_id"`
}

func loadMessageStore() (*messageStore, error) {
	data, err := os.ReadFile(messagesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &messageStore{Messages: []storedMessage{}}, nil
		}
		return nil, err
	}

	var store messageStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return &store, nil
}

func (s *messageStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(messagesFile, data, 0o644)
}

func (s *messageStore) getMessageID(categoryName string, messageIndex int) (snowflake.ID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, msg := range s.Messages {
		if msg.CategoryName == categoryName && msg.MessageIndex == messageIndex {
			return msg.MessageID, true
		}
	}
	return 0, false
}

func (s *messageStore) setMessageID(categoryName string, messageIndex int, messageID snowflake.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update existing
	for i, msg := range s.Messages {
		if msg.CategoryName == categoryName && msg.MessageIndex == messageIndex {
			s.Messages[i].MessageID = messageID
			return
		}
	}

	// Add new
	s.Messages = append(s.Messages, storedMessage{
		CategoryName: categoryName,
		MessageIndex: messageIndex,
		MessageID:    messageID,
	})
}
