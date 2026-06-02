package auction

import "time"

const (
	StateDraft  = "DRAFT"
	StateOpen   = "OPEN"
	StateClosed = "CLOSED"
)

func CanTransition(current, next string, now, start, end time.Time) bool {
	switch current {
	case StateDraft:
		if next == StateOpen {
			return !now.Before(start) && now.Before(end)
		}
	case StateOpen:
		if next == StateClosed {
			return true
		}
	}
	return false
}

func IsOpen(state string, now, start, end time.Time) bool {
	return state == StateOpen && !now.Before(start) && now.Before(end)
}
