package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
	"github.com/tez-capital/tezpay/common"
	"github.com/tez-capital/tezpay/constants"
)

type (
	SlackBotNotificator struct {
		token   string
		channel string
	}
	slackMessage struct {
		Channel  string `json:"channel"`
		Text     string `json:"text"`
		ThreadTS string `json:"thread_ts,omitempty"`
	}
	slackBotNotificatorConfiguration struct {
		Channel string `json:"channel"`
		Token   string `json:"token"`
	}
)

func InitSlackBotNotificator(configurationBytes []byte) (*SlackBotNotificator, error) {
	configuration := slackBotNotificatorConfiguration{}
	err := json.Unmarshal(configurationBytes, &configuration)
	if err != nil {
		return nil, err
	}

	slog.Debug("slack bot notificator initialized")

	return &SlackBotNotificator{
		token:   configuration.Token,
		channel: configuration.Channel,
	}, nil
}

func ValidateSlackBotConfiguration(configurationBytes []byte) error {
	configuration := slackBotNotificatorConfiguration{}
	err := json.Unmarshal(configurationBytes, &configuration)
	if err != nil {
		return err
	}
	if configuration.Channel == "" {
		return errors.Wrap(constants.ErrInvalidNotificatorConfiguration, "invalid url")
	}
	if configuration.Token == "" {
		return errors.Wrap(constants.ErrInvalidNotificatorConfiguration, "invalid token")
	}
	return nil
}

func (s SlackBotNotificator) send(message, threadTS string) (treadID string, err error) {
	m := slackMessage{
		Channel:  s.channel,
		Text:     message,
		ThreadTS: threadTS,
	}

	payload, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %v", err)
	}

	url := "https://slack.com/api/chat.postMessage"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send message to Slack: %v", err)
	}
	defer resp.Body.Close()

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if !response["ok"].(bool) {
		return "", fmt.Errorf("Slack API error: %v", response["error"])
	}

	if threadTS, ok := response["ts"].(string); ok {
		return threadTS, nil
	}
	return "", nil
}

func (s SlackBotNotificator) PayoutSummaryNotify(summary *common.CyclePayoutSummary, additionalData map[string]string) error {
	threadMessage := fmt.Sprintf(":white_check_mark: TEZOS mainnet - cycle %d", summary.Cycle)
	subMessage := fmt.Sprintf(
		"Delegators: %d\nPaid Delegators: %d\nOwn Staked Balance: %s XTZ\nOwn Delegated Balance: %s XTZ\nExternal Staked Balance: %s XTZ\nExternal Delegated Balance: %s XTZ\nCycle Fees: %s XTZ\nCycle Rewards: %s XTZ\nDistributed Rewards: %s XTZ\nTransaction Fees Paid: %s XTZ\nBond Income: %s XTZ\nFee Income: %s XTZ\nTotal Income: %s XTZ\nDonated Bonds: %s XTZ\nDonated Fees: %s XTZ\nDonated Total: %s XTZ\nTimestamp: %s\n",
		summary.Delegators, summary.PaidDelegators,
		parseAndScale(summary.OwnStakedBalance.String()),
		parseAndScale(summary.OwnDelegatedBalance.String()),
		parseAndScale(summary.ExternalStakedBalance.String()),
		parseAndScale(summary.ExternalDelegatedBalance.String()),
		parseAndScale(summary.EarnedFees.String()),
		parseAndScale(summary.EarnedRewards.String()),
		parseAndScale(summary.DistributedRewards.String()),
		parseAndScale(summary.TransactionFeesPaid.String()),
		parseAndScale(summary.BondIncome.String()),
		parseAndScale(summary.FeeIncome.String()),
		parseAndScale(summary.IncomeTotal.String()),
		parseAndScale(summary.DonatedBonds.String()),
		parseAndScale(summary.DonatedFees.String()),
		parseAndScale(summary.DonatedTotal.String()),
		summary.Timestamp,
	)
	threadTS, err := s.send(threadMessage, "")
	if err != nil {
		return errors.Wrap(err, "send(main)")
	}

	_, err = s.send(subMessage, threadTS)
	if err != nil {
		return errors.Wrap(err, "send(sub)")
	}

	return nil
}

func (s SlackBotNotificator) AdminNotify(msg string) error {
	_, err := s.send(msg, "")
	if err != nil {
		return errors.Wrap(err, "send()")
	}
	return nil
}

func (s SlackBotNotificator) TestNotify() error {
	_, err := s.send("test slack bot message", "")
	if err != nil {
		return errors.Wrap(err, "send")
	}
	return nil
}

func parseAndScale(value string) string {
	parsedValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "Invalid"
	}
	scaledValue := parsedValue / 1e6
	return fmt.Sprintf("%.6f", scaledValue)
}
