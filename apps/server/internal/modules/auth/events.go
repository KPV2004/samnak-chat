package auth

// DeviceRevoked is published after a device's sessions were revoked (AUTH-08):
// by a logout, by revoking one device, or by logging out every device of the
// user. It is published after the write committed, so consumers never close a
// connection for a revocation that was rolled back.
//
// An empty DeviceID means every device of the user was revoked at once. An
// event is only published when something was actually revoked, so replaying a
// revoke announces nothing.
type DeviceRevoked struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id,omitempty"`
}

func (DeviceRevoked) EventName() string { return "auth.device_revoked" }
