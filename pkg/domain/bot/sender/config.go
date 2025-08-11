package sender

import (
	"a/GO/tg_services_bot-main/pkg/utils/errs"
	"os"

	"github.com/joho/godotenv"
)

// ProcessorConfig values should be loaded from .env file.
type ProcessorConfig struct {
	Token     string
	channelID string
}

func (c *ProcessorConfig) LoadFromEnv() error {
	if err := godotenv.Load(); err != nil {
		return errs.New("failed to load .env").Wrap(err)
	}

	token := os.Getenv("TG_TOKEN")
	if token == "" {
		return errs.New("empty token")
	}

	channelID := os.Getenv("TG_CHANNEL_ID")
	if channelID == "" {
		return errs.New("empty channel id")
	}

	c.Token = token
	c.channelID = channelID

	return nil
}
