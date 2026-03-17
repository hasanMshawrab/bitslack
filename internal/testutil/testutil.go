// Package testutil provides mock implementations of the bitslack adapter
// interfaces for use in tests.
package testutil

import (
	"context"
	"sync"
)

// MockThreadStore is a test double for bitslack.ThreadStore.
type MockThreadStore struct {
	mu       sync.Mutex
	store    map[string]string
	GetCalls []string // prKeys passed to Get
	SetCalls []struct{ Key, TS string }
	GetErr   error // if non-nil, Get returns this error
	SetErr   error // if non-nil, Store returns this error
}

func NewMockThreadStore() *MockThreadStore {
	return &MockThreadStore{store: make(map[string]string)}
}

func (m *MockThreadStore) Get(_ context.Context, prKey string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetCalls = append(m.GetCalls, prKey)
	if m.GetErr != nil {
		return "", false, m.GetErr
	}
	ts, ok := m.store[prKey]
	return ts, ok, nil
}

func (m *MockThreadStore) Store(_ context.Context, prKey string, ts string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetCalls = append(m.SetCalls, struct{ Key, TS string }{prKey, ts})
	if m.SetErr != nil {
		return m.SetErr
	}
	m.store[prKey] = ts
	return nil
}

// Seed pre-populates the store for tests with existing threads.
func (m *MockThreadStore) Seed(prKey, ts string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[prKey] = ts
}

// MockConfigStore is a test double for bitslack.ConfigStore.
type MockConfigStore struct {
	Channels map[string]string // repo full name -> channel ID
	Users    map[string]string // bitbucket username -> slack user ID
}

func NewMockConfigStore() *MockConfigStore {
	return &MockConfigStore{
		Channels: map[string]string{
			"myworkspace/my-repo": "C001ENG",
		},
		Users: map[string]string{
			// Keyed by Bitbucket account_id (stable across sources).
			"5b10a2844c20165700ede22h": "U001JANE",  // janeauthor
			"5b10a2844c20165700ede23i": "U002BOB",   // bobreviewer
			"5b10a2844c20165700ede24j": "U003ALICE", // alicereviewer
		},
	}
}

func (m *MockConfigStore) GetChannel(repo string) (string, bool) {
	ch, ok := m.Channels[repo]
	return ch, ok
}

func (m *MockConfigStore) GetSlackUserID(username string) (string, bool) {
	id, ok := m.Users[username]
	return id, ok
}

// MockLogger captures log messages for assertion.
type MockLogger struct {
	mu        sync.Mutex
	InfoMsgs  []string
	WarnMsgs  []string
	ErrorMsgs []string
}

func (m *MockLogger) Info(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InfoMsgs = append(m.InfoMsgs, msg)
}

func (m *MockLogger) Warn(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WarnMsgs = append(m.WarnMsgs, msg)
}

func (m *MockLogger) Error(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorMsgs = append(m.ErrorMsgs, msg)
}
