package samsung

import "errors"

var (
	ErrUnauthorized      = errors.New("samsung: TV rejected authorization — accept the prompt on the TV screen")
	ErrTimeout           = errors.New("samsung: operation timed out")
	ErrNotConnected      = errors.New("samsung: not connected")
	ErrConnectionFailure = errors.New("samsung: connection handshake failed")
	ErrArtAPIError       = errors.New("samsung: art API returned an error")
	ErrStorageFull       = errors.New("samsung: TV storage is full")
)
