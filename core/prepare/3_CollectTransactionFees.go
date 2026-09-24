package prepare

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/samber/lo"
	"github.com/tez-capital/tezpay/common"
	"github.com/tez-capital/tezpay/constants/enums"
	"github.com/tez-capital/tezpay/core/estimate"
	"github.com/tez-capital/tezpay/utils"
)

const (
	maxEstimateErrorLengthInAdminNotification = 150
	// keeps the message within the smallest common notificator limit (discord - 2000 chars)
	maxEstimateFailuresAdminNotificationLength = 1900
)

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

func formatEstimateFailuresAdminNotification(cycles []int64, failures []estimate.EstimateResult[*common.AccumulatedPayoutRecipe]) string {
	msg := fmt.Sprintf("⚠️ Failed to estimate tx costs for %d payout(s) in %s - they were excluded from this payout run (%s):", len(failures), utils.FormatCycleNumbers(cycles...), enums.INVALID_FAILED_TO_ESTIMATE_TX_COSTS)
	for i, failure := range failures {
		tx := failure.Transaction
		errMsg := truncateRunes(strings.ReplaceAll(failure.Error.Error(), "\n", "; "), maxEstimateErrorLengthInAdminNotification)
		line := fmt.Sprintf("\n- delegator %s -> recipient %s, %s %s: %s", tx.Delegator, tx.Recipient, common.MutezToTezS(tx.GetAmount().Int64()), tx.TxKind, errMsg)
		// reserve space for the "and N more" suffix
		if utf8.RuneCountInString(msg)+utf8.RuneCountInString(line)+40 > maxEstimateFailuresAdminNotificationLength {
			msg += fmt.Sprintf("\n... and %d more (see logs)", len(failures)-i)
			break
		}
		msg += line
	}
	return msg
}

func CollectTransactionFees(ctx *PayoutPrepareContext, options *common.PreparePayoutsOptions) (result *PayoutPrepareContext, err error) {
	logger := ctx.logger.With("phase", "collect_transaction_fees")
	logger.Info("collecting transaction fees")

	estimateContext := &estimate.EstimationContext{
		PayoutKey:                            ctx.PayoutKey,
		Collector:                            ctx.GetCollector(),
		Configuration:                        ctx.configuration,
		BatchMetadataDeserializationGasLimit: ctx.StageData.BatchMetadataDeserializationGasLimit,
	}

	validAccumulatedRecipes := utils.OnlyValidAccumulatedPayouts(ctx.StageData.AccumulatedPayouts)
	// get new estimates
	estimateFailures := make([]estimate.EstimateResult[*common.AccumulatedPayoutRecipe], 0)
	recipesWithEstimate := lo.Map(estimate.EstimateTransactionFees(validAccumulatedRecipes, estimateContext), func(result estimate.EstimateResult[*common.AccumulatedPayoutRecipe], _ int) *common.AccumulatedPayoutRecipe {
		if result.Error != nil {
			logger.Warn("failed to estimate tx costs", "recipient", result.Transaction.Recipient, "delegator", result.Transaction.Delegator, "payout_address", ctx.PayoutKey.Address(), "amount", result.Transaction.GetAmount().Int64(), "kind", result.Transaction.TxKind, "error", result.Error)
			estimateFailures = append(estimateFailures, result)
			result.Transaction.IsValid = false
			result.Transaction.Note = string(enums.INVALID_FAILED_TO_ESTIMATE_TX_COSTS)
			return result.Transaction
		}

		recipe := result.Transaction
		backup := recipe.DeepClone()
		recipe.OpLimits = result.OpLimits

		isBakerPayingTxFee := ctx.configuration.PayoutConfiguration.IsPayingTxFee
		isBakerPayingAllocationTxFee := ctx.configuration.PayoutConfiguration.IsPayingAllocationTxFee

		delegatorOverrides := ctx.configuration.Delegators.Overrides
		if delegatorOverride, ok := delegatorOverrides[recipe.Delegator.String()]; ok {
			if delegatorOverride.IsBakerPayingTxFee != nil {
				isBakerPayingTxFee = *delegatorOverride.IsBakerPayingTxFee
			}
			if delegatorOverride.IsBakerPayingAllocationTxFee != nil {
				isBakerPayingAllocationTxFee = *delegatorOverride.IsBakerPayingAllocationTxFee
			}
		}

		bondsAmountBeforeFees := recipe.GetAmount()
		utils.AssertZAmountPositiveOrZero(bondsAmountBeforeFees)

		txFee := result.OpLimits.GetOperationFeesWithoutAllocation()
		allocationFee := result.OpLimits.GetAllocationFee()

		// only tez delegator rewards are subject to fee collection
		recipe.AddTxFee64(txFee, recipe.TxKind == enums.PAYOUT_TX_KIND_TEZ && recipe.Kind == enums.PAYOUT_KIND_DELEGATOR_REWARD && !isBakerPayingTxFee)
		recipe.AddTxFee64(allocationFee, recipe.TxKind == enums.PAYOUT_TX_KIND_TEZ && recipe.Kind == enums.PAYOUT_KIND_DELEGATOR_REWARD && !isBakerPayingAllocationTxFee)
		if recipe.GetAmount().IsNeg() || recipe.GetAmount().IsZero() {
			recipe = backup // restore as we wont charge fees if we are invalid
			recipe.IsValid = false
			recipe.Note = string(enums.INVALID_NOT_ENOUGH_BONDS_FOR_TX_FEES)
		}
		utils.AssertZAmountPositiveOrZero(recipe.GetAmount())
		return recipe
	})

	if len(estimateFailures) > 0 {
		cycles := lo.Uniq(lo.Map(ctx.PayoutBlueprints, func(blueprint *common.CyclePayoutBlueprint, _ int) int64 { return blueprint.Cycle }))
		ctx.AdminNotify(formatEstimateFailuresAdminNotification(cycles, estimateFailures))
	}

	ctx.StageData.AccumulatedPayouts = utils.OnlyValidAccumulatedPayouts(recipesWithEstimate) // overwrite with new only valid ones

	newInvalidAccumulatedRecipes := utils.OnlyInvalidAccumulatedPayouts(recipesWithEstimate)
	newInvalidRecipes := lo.Flatten(lo.Map(newInvalidAccumulatedRecipes, func(recipe *common.AccumulatedPayoutRecipe, _ int) []common.PayoutRecipe {
		return recipe.DisperseToInvalid()
	}))
	ctx.StageData.InvalidRecipes = append(ctx.StageData.InvalidRecipes, newInvalidRecipes...)
	return ctx, nil
}
