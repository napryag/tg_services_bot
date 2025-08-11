package sender

import (
	"a/GO/tg_services_bot-main/pkg/utils/errs"
	"math"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type Processor struct {
	config ProcessorConfig
	logger zerolog.Logger

	bot *tgbotapi.BotAPI
}

func New(config ProcessorConfig, logger zerolog.Logger, bot *tgbotapi.BotAPI) *Processor {
	return &Processor{
		config: config,
		logger: logger,
		bot:    bot,
	}
}

func (p *Processor) Send(text string) (int, error) {
	p.logger.Trace().Msg("In")
	defer p.logger.Trace().Msg("Out")

	msgToSend := tgbotapi.NewMessageToChannel(p.config.channelID, text)

	var err error
	var msg tgbotapi.Message

	for i := 0; i < 3; i++ {
		msg, err = p.bot.Send(msgToSend)
		if err == nil {
			return msg.MessageID, nil
		}
		p.logger.Warn().Err(err).Int("retry", i+1).Msg("send failed, retrying")

		if i != 0 {
			time.Sleep(time.Duration(math.Pow(2, float64(i))) * time.Second)
		}
	}
	p.logger.Error().Err(err).Msg("send permanently failed")

	return 0, errs.New("failed to send message").Wrap(err)

}
