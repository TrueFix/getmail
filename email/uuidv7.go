package email

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// UUIDv7 represents a UUID version 7
type UUIDv7 struct {
	bytes [16]byte
}

// String returns the UUID as a formatted string
func (u UUIDv7) String() string {
	b := u.bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16])
}

// NewUUIDv7 generates a new UUIDv7
func NewUUIDv7() (UUIDv7, error) {
	var value [16]byte
	_, err := rand.Read(value[:])
	if err != nil {
		return UUIDv7{}, err
	}

	// Get current timestamp in milliseconds
	timestamp := big.NewInt(time.Now().UnixMilli())

	// Fill first 6 bytes with timestamp
	timestamp.FillBytes(value[0:6])

	// Set version (7)
	value[6] = (value[6] & 0x0F) | 0x70

	// Set variant (10xxxxxx)
	value[8] = (value[8] & 0x3F) | 0x80

	return UUIDv7{bytes: value}, nil
}
