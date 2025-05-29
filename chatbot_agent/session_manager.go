// chatbot_agent/session_manager.go
package main

import (
	"sync"

	pb "github.com/pmartinizquierdob/mcp-google-services/pb"
)

var (
	userSessions  = make(map[string]*UserSession)
	sessionsMutex sync.Mutex
)

// GetOrCreateUserSession retrieves an existing session or creates a new one.
func GetOrCreateUserSession(userID string) (*UserSession, bool) {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	session, ok := userSessions[userID]
	if !ok {
		session = &UserSession{} // Initialize with empty values, will be filled later
		userSessions[userID] = session
	}
	return session, ok
}

// UpdateUserSessionTokens updates the OAuth tokens for a user session.
func UpdateUserSessionTokens(userID string, tokens *pb.OAuthTokens) {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()
	if session, ok := userSessions[userID]; ok {
		session.OAuthTokens = tokens
	}
}
