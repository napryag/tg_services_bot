package config

import (
	"os"
	"path/filepath"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/napryag/tg_services_bot/pkg/utils/errs"
	"gopkg.in/yaml.v3"
)

type Config struct {
	PostgreAddr string `yaml:"postgre_addr" validate:"required"`
	// WebhookURL  string `yaml:"webhook_url" validate:"required"`
	HTTPPort    int `yaml:"http_port" validate:"required"`
	WorkerCount int `yaml:"worker_count" validate:"required"`
	BotToken    string
}

func LoadConfig() (*Config, error) {
	path := filepath.Join("cmd/bot/etc", "app.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New("failed to read config file").Wrap(err)
	}

	var cfg Config
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errs.New("failed to unmarshal YAML").Wrap(err)
	}

	// Validate
	if err = validator.New().Struct(cfg); err != nil {

		return nil, errs.New("config validation failed").Wrap(err)
	}

	if err = godotenv.Load(); err != nil {
		return nil, errs.New("failed to load .env").Wrap(err)
	}
	cfg.BotToken = os.Getenv("TG_TOKEN")

	return &cfg, nil
}
