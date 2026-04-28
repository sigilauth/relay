package push

import "context"

// PushProvider abstracts push notification delivery to enable testing
// and support for multiple platforms (APNs, FCM).
type PushProvider interface {
	// Send delivers a notification to the specified push token.
	// Returns error if delivery fails (network, invalid token, etc).
	Send(ctx context.Context, token string, payload []byte) error

	// Platform returns the platform identifier ("apns" or "fcm").
	Platform() string
}

// PushToken represents a registered device's push notification token.
type PushToken struct {
	Fingerprint      string
	Token            string
	Platform         string
	RegisteredAt     int64
	UpdatedAt        int64
	LastDeliveredAt  *int64
	DeliveryFailures int
}

// Result captures the outcome of a push delivery attempt.
type Result struct {
	Fingerprint string
	Delivered   bool
	Platform    string
	Error       error
}
