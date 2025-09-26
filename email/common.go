package email

import (
	"log"
)

func LogError(context string, err error) {
	log.Printf("[ERROR] %s: %v", context, err)
}

func LogInfo(context string, msg string) {
	log.Printf("[INFO] %s: %s", context, msg)
}

func LogWarning(context string, msg string) {
	log.Printf("[WARNING] %s: %s", context, msg)
}
