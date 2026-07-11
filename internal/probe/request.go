package probe

import "time"

type Request struct {
	ID        uint64
	SessionID uint64
	GroupID   uint8
	Target    string
	SentAt    time.Time
	Deadline  time.Time
}
