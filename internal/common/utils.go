package common

import "time"

func GetCurrentTimestamp() string {
	timestamp := time.Now().Format(time.RFC3339)
	return timestamp
}
