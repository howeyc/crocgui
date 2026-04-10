//go:build android || ios

package main

// goSenderActive is always false on mobile
var goSenderActive bool

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

// handleLocalPeerSettingsForGoSender is a no-op on mobile
func handleLocalPeerSettingsForGoSender(msg map[string]interface{}, roomID, peerID string, room *VideoCallRoom) {
}
