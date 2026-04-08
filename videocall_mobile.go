//go:build android || ios

package main

// goSenderActive is always false on mobile
var goSenderActive bool

// goSenderPeerID is unused on mobile
var goSenderPeerID string

// isGoSenderAvailable — на мобильных браузер сам отправляет медиа
const isGoSenderAvailable = false

// startGoSender is a no-op on mobile
func startGoSender(roomID, peerID string, room *VideoCallRoom) error {
	return nil
}

// stopGoSender is a no-op on mobile
func stopGoSender() {}

// handlePeerSettingsForGoSender is a no-op on mobile
func handlePeerSettingsForGoSender(msg map[string]interface{}) {}

// handleRestartRecorderForGoSender is a no-op on mobile
func handleRestartRecorderForGoSender() {}
