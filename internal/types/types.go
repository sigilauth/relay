package types

// RegistrationChallengeResponse is returned from GET /devices/register/challenge
type RegistrationChallengeResponse struct {
	ChallengeID string `json:"challenge_id"`
	Nonce       string `json:"nonce"` // base64-encoded 32-byte random nonce
}

// DeviceRegisterRequest matches OpenAPI schema DeviceRegister
// Now requires proof of key ownership via signed challenge
type DeviceRegisterRequest struct {
	DevicePublicKey string `json:"device_public_key"`
	PushToken       string `json:"push_token"`
	PushPlatform    string `json:"push_platform"` // "apns" or "fcm"
	ChallengeID     string `json:"challenge_id"`
	Signature       string `json:"signature"` // base64-encoded ECDSA signature
}

// DeviceRegisterResponse matches OpenAPI schema DeviceRegistered
type DeviceRegisterResponse struct {
	Fingerprint        string   `json:"fingerprint"`
	Pictogram          []string `json:"pictogram"`
	PictogramSpeakable string   `json:"pictogram_speakable"`
	RegisteredAt       string   `json:"registered_at"` // ISO8601
}

// PushRequest matches OpenAPI schema PushRequest
type PushRequest struct {
	ServerID         string                 `json:"server_id"`
	Fingerprint      string                 `json:"fingerprint"`
	Payload          map[string]interface{} `json:"payload"`
	Timestamp        string                 `json:"timestamp"` // ISO8601
	RequestSignature string                 `json:"request_signature"`
}

// PushResponse matches OpenAPI schema PushResult
type PushResponse struct {
	Status      string `json:"status"` // "delivered", "queued", "failed"
	PushID      string `json:"push_id,omitempty"`
	DeliveredAt string `json:"delivered_at,omitempty"` // ISO8601
}

// ErrorResponse matches OpenAPI schema Error
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}
