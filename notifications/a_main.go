package notifications

import (
	"errors"
	"fmt"

	"github.com/tez-capital/tezpay/common"
	"github.com/tez-capital/tezpay/constants"
)

func LoadNotificatior(kind NotificatorKind, configuration []byte) (common.NotificatorEngine, error) {
	switch kind {
	case TWITTER_NOTIFICATOR:
		return InitTwitterNotificator(configuration)
	case DISCORD_NOTIFICATOR:
		return InitDiscordNotificator(configuration)
	case TELEGRAM_NOTIFICATOR:
		return InitTelegramNotificator(configuration)
	case EMAIL_NOTIFICATOR:
		return InitEmailNotificator(configuration)
	case EXTERNAL_NOTIFICATOR:
		return InitExternalNotificator(configuration)
	case WEBHOOK_NOTIFICATOR:
		return InitWebhookNotificator(configuration)
	case BLUESKY_NOTIFICATOR:
		return InitBlueskyNotificator(configuration)
	case SLACK_BOT_NOTIFICATOR:
		return InitSlackBotNotificator(configuration)
	default:
		return nil, errors.Join(constants.ErrUnsupportedNotificator, fmt.Errorf("kind: %s", kind))
	}
}

func ValidateNotificatorConfiguration(kind NotificatorKind, configuration []byte) error {
	switch kind {
	case TWITTER_NOTIFICATOR:
		return ValidateTwitterConfiguration(configuration)
	case DISCORD_NOTIFICATOR:
		return ValidateDiscordConfiguration(configuration)
	case TELEGRAM_NOTIFICATOR:
		return ValidateTelegramConfiguration(configuration)
	case EMAIL_NOTIFICATOR:
		return ValidateEmailConfiguration(configuration)
	case NotificatorKind(WEBHOOK_NOTIFICATOR):
		return ValidateWebhookConfiguration(configuration)
	case EXTERNAL_NOTIFICATOR:
		return ValidateExternalConfiguration(configuration)
	case BLUESKY_NOTIFICATOR:
		return ValidateBlueskyConfiguration(configuration)
	case SLACK_BOT_NOTIFICATOR:
		return ValidateSlackBotConfiguration(configuration)
	default:
		return errors.Join(constants.ErrUnsupportedNotificator, fmt.Errorf("kind: %s", kind))
	}
}
