package reports

import (
	"time"

	"github.com/google/uuid"
)

// kind values
const (
	KindLost  = "lost"
	KindFound = "found"
)

// status values
const (
	StatusActive   = "active"
	StatusMatched  = "matched"
	StatusResolved = "resolved"
	StatusExpired  = "expired"
)

// Report is a lost- or found-item report. Location is optional; when present it
// is a WGS84 lat/lng pair.
type Report struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Kind        string
	Category    string
	Description string
	Lat         float64
	Lng         float64
	HasLocation bool
	EventAt     *time.Time
	Status      string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// Detail is one finder-provided private verification fact, in plaintext. It is
// only ever held in memory; at rest it lives encrypted (see EncryptedDetail).
// ID is zero when the detail is an input (not yet persisted).
type Detail struct {
	ID    uuid.UUID
	Kind  string // e.g. "amount", "location_detail"
	Value string
}

// EncryptedDetail is the at-rest form stored in verification_details.
type EncryptedDetail struct {
	ID         uuid.UUID
	Kind       string
	Ciphertext []byte
	Nonce      []byte
}

// SubmitInput is the validated payload for creating a report.
type SubmitInput struct {
	UserID      uuid.UUID
	Kind        string
	Category    string
	Description string
	Lat         float64
	Lng         float64
	HasLocation bool
	EventAt     *time.Time
	Details     []Detail // finder only
}
