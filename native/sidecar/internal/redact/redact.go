package redact

import (
	"os"
	"strings"
)

func FromEnv(text string) string {
	return Secrets(
		text,
		os.Getenv("HF_TOKEN"),
		os.Getenv("HUGGING_FACE_HUB_TOKEN"),
	)
}

func Secrets(text string, secrets ...string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}

	return text
}
