package prepare

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/tez-capital/tezpay/common"
	"github.com/tez-capital/tezpay/configuration"
	"github.com/tez-capital/tezpay/constants"
	"github.com/tez-capital/tezpay/constants/enums"
	"github.com/tez-capital/tezpay/core/estimate"
	"github.com/tez-capital/tezpay/test/mock"
	"github.com/trilitech/tzgo/tezos"
)

var (
	payoutRecipes = getRecipes()
)

func getRecipes() []*common.AccumulatedPayoutRecipe {
	return []*common.AccumulatedPayoutRecipe{
		{
			Delegator: mock.GetRandomAddress(),
			Recipient: mock.GetRandomAddress(),
			Kind:      enums.PAYOUT_KIND_DELEGATOR_REWARD,
			TxKind:    enums.PAYOUT_TX_KIND_TEZ,
			IsValid:   true,
			Recipes: []*common.PayoutRecipe{
				{
					Delegator: mock.GetRandomAddress(),
					Recipient: mock.GetRandomAddress(),
					Amount:    tezos.NewZ(10000000),
					Kind:      enums.PAYOUT_KIND_DELEGATOR_REWARD,
					TxKind:    enums.PAYOUT_TX_KIND_TEZ,
					IsValid:   true,
				},
			},
		},
		{
			Delegator: mock.GetRandomAddress(),
			Recipient: mock.GetRandomAddress(),
			Kind:      enums.PAYOUT_KIND_DELEGATOR_REWARD,
			TxKind:    enums.PAYOUT_TX_KIND_TEZ,
			IsValid:   true,
			Recipes: []*common.PayoutRecipe{
				{
					Delegator: mock.GetRandomAddress(),
					Recipient: mock.GetRandomAddress(),
					Amount:    tezos.NewZ(20000000),
					Kind:      enums.PAYOUT_KIND_DELEGATOR_REWARD,
					TxKind:    enums.PAYOUT_TX_KIND_TEZ,
					IsValid:   true,
				},
			},
		},
	}
}

func TestCollectTransactionFees(t *testing.T) {
	var result *PayoutPrepareContext
	var err error
	config := configuration.GetDefaultRuntimeConfiguration()
	collector := mock.InitSimpleCollector()
	signer := mock.InitSimpleSigner()

	assert := assert.New(t)
	ctx := &PayoutPrepareContext{
		PreparePayoutsEngineContext: *common.NewPreparePayoutsEngineContext(collector, signer, nil, func(msg string) {}),
		StageData:                   &StageData{AccumulatedPayouts: getRecipes()},
		configuration:               &config,

		logger: slog.Default(),
	}

	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 0,
		StorageBurn:    0,
		UsedMilliGas:   2000000,
	})
	result, err = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})

	assert.Nil(err)
	for i, v := range result.StageData.AccumulatedPayouts {
		assert.LessOrEqual(v.GetAmount().Int64()-constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.GreaterOrEqual(v.GetAmount().Int64()+constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.Equal(v.OpLimits.GetOperationTotalFees(), v.GetTxFee())
	}

	t.Log("check allocation burn")
	ctx.StageData.AccumulatedPayouts = getRecipes()
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   2000000,
	})
	result, err = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})

	assert.Nil(err)
	for i, v := range result.StageData.AccumulatedPayouts {
		assert.LessOrEqual(v.GetAmount().Int64()-constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.GreaterOrEqual(v.GetAmount().Int64()+constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.Equal(v.OpLimits.GetOperationTotalFees(), v.GetTxFee())
	}

	t.Log("check storage burn")
	ctx.StageData.AccumulatedPayouts = getRecipes()
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   1000000,
	})
	result, err = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})

	assert.Nil(err)
	for i, v := range result.StageData.AccumulatedPayouts {
		assert.LessOrEqual(v.GetAmount().Int64()-constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.GreaterOrEqual(v.GetAmount().Int64()+constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.Equal(v.OpLimits.GetOperationTotalFees(), v.GetTxFee())
	}

	t.Log("check paying tx fee")
	ctx.StageData.AccumulatedPayouts = getRecipes()
	ctx.configuration.PayoutConfiguration.IsPayingTxFee = true
	ctx.configuration.PayoutConfiguration.IsPayingAllocationTxFee = false
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   1000000,
	})
	result, _ = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	for i, v := range result.StageData.AccumulatedPayouts {
		assert.Equal(v.GetAmount().Int64(), payoutRecipes[i].GetAmount().Int64()-1000 /*allocation fee*/)
		assert.Equal(v.OpLimits.GetOperationTotalFees(), v.GetTxFee())
	}

	t.Log("check not paying tx & allocation fee")
	ctx.StageData.AccumulatedPayouts = getRecipes()
	ctx.configuration.PayoutConfiguration.IsPayingTxFee = true
	ctx.configuration.PayoutConfiguration.IsPayingAllocationTxFee = true
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   1000000,
	})
	result, _ = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	for i, v := range result.StageData.AccumulatedPayouts {
		assert.Equal(v.GetAmount().Int64(), payoutRecipes[i].GetAmount().Int64())
		assert.Equal(v.OpLimits.GetOperationTotalFees(), v.GetTxFee())
	}

	t.Log("check per op estimate")
	ctx.StageData.AccumulatedPayouts = getRecipes()
	ctx.configuration.PayoutConfiguration.IsPayingTxFee = false
	ctx.configuration.PayoutConfiguration.IsPayingAllocationTxFee = false
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   1000000,
		SingleOnly:     true,
	})
	result, err = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})

	assert.Nil(err)
	for i, v := range result.StageData.AccumulatedPayouts {
		assert.LessOrEqual(v.GetAmount().Int64()-constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.GreaterOrEqual(v.GetAmount().Int64()+constants.TEST_MUTEZ_DEVIATION_TOLERANCE, payoutRecipes[i].GetAmount().Int64()-collector.GetExpectedTxCosts())
		assert.Equal(v.OpLimits.GetOperationTotalFees(), v.GetTxFee())
	}

	t.Log("check batching")
	ctx.StageData.AccumulatedPayouts = getRecipes()
	ctx.configuration.PayoutConfiguration.IsPayingTxFee = false
	ctx.configuration.PayoutConfiguration.IsPayingAllocationTxFee = false
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   1000000,
		SingleOnly:     true,
	})
	ops := []*common.AccumulatedPayoutRecipe{}
	for len(ops) < constants.DEFAULT_SIMULATION_TX_BATCH_SIZE*2.5 {
		ops = append(ops, getRecipes()...)
	}

	ctx.StageData.AccumulatedPayouts = ops
	result, err = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	assert.Nil(err)
	assert.Equal(len(ops), len(result.StageData.AccumulatedPayouts))

	ctx.StageData.AccumulatedPayouts = getRecipes()

	t.Log("fail estimate")
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn: 1000,
		StorageBurn:    0,
		UsedMilliGas:   1000000,
		SingleOnly:     true,
		FailWithError:  errors.New("failed estimate"),
	})
	ctx.StageData.AccumulatedPayouts = getRecipes()
	result, _ = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	for _, v := range result.StageData.AccumulatedPayouts {
		assert.Equal(v.IsValid, false)
		assert.Equal(v.Note, enums.INVALID_FAILED_TO_ESTIMATE_TX_COSTS)
	}

	t.Log("failed receipt")
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn:       1000,
		StorageBurn:          0,
		UsedMilliGas:         1000000,
		SingleOnly:           true,
		FailWithReceiptError: errors.New("failed receipt"),
	})
	ctx.StageData.AccumulatedPayouts = getRecipes()
	result, _ = CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	for _, v := range result.StageData.AccumulatedPayouts {
		assert.Equal(v.IsValid, false)
		assert.Equal(v.Note, enums.INVALID_FAILED_TO_ESTIMATE_TX_COSTS)
	}

	t.Log("test partial panic")
	collector.SetOpts(&mock.SimpleCollectorOpts{
		AllocationBurn:   1000,
		StorageBurn:      0,
		UsedMilliGas:     1000000,
		SingleOnly:       false,
		ReturnOnlyNCosts: 1,
	})
	ctx.StageData.AccumulatedPayouts = getRecipes()
	assert.Panics(func() {
		_, err := CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
		t.Log(err)
	})
}

func TestCollectTransactionFeesNotifiesAdminOnEstimateFailure(t *testing.T) {
	assert := assert.New(t)
	config := configuration.GetDefaultRuntimeConfiguration()
	collector := mock.InitSimpleCollector()
	signer := mock.InitSimpleSigner()

	notifications := []string{}
	ctx := &PayoutPrepareContext{
		PreparePayoutsEngineContext: *common.NewPreparePayoutsEngineContext(collector, signer, nil, func(msg string) {
			notifications = append(notifications, msg)
		}),
		StageData:        &StageData{AccumulatedPayouts: getRecipes()},
		PayoutBlueprints: []*common.CyclePayoutBlueprint{{Cycle: 1000}},
		configuration:    &config,
		logger:           slog.Default(),
	}

	t.Log("no notification on success")
	collector.SetOpts(&mock.SimpleCollectorOpts{UsedMilliGas: 1000000})
	_, err := CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	assert.Nil(err)
	assert.Empty(notifications)

	t.Log("single aggregated notification on failure")
	recipes := getRecipes()
	ctx.StageData.AccumulatedPayouts = recipes
	collector.SetOpts(&mock.SimpleCollectorOpts{
		UsedMilliGas:  1000000,
		FailWithError: errors.New("script_rejected"),
	})
	result, err := CollectTransactionFees(ctx, &common.PreparePayoutsOptions{})
	assert.Nil(err)
	assert.Empty(result.StageData.AccumulatedPayouts)
	assert.Len(notifications, 1)
	assert.Contains(notifications[0], "2 payout(s)")
	assert.Contains(notifications[0], "1000")
	assert.Contains(notifications[0], "script_rejected")
	for _, recipe := range recipes {
		assert.Contains(notifications[0], recipe.Delegator.String())
		assert.Contains(notifications[0], recipe.Recipient.String())
	}
}

func TestFormatEstimateFailuresAdminNotificationIsCapped(t *testing.T) {
	assert := assert.New(t)
	failures := make([]estimate.EstimateResult[*common.AccumulatedPayoutRecipe], 0)
	for i := 0; i < 50; i++ {
		failures = append(failures, estimate.EstimateResult[*common.AccumulatedPayoutRecipe]{
			Transaction: getRecipes()[0],
			Error:       errors.New(strings.Repeat("ž", maxEstimateErrorLengthInAdminNotification*2)),
		})
	}
	msg := formatEstimateFailuresAdminNotification([]int64{1000}, failures)
	assert.LessOrEqual(utf8.RuneCountInString(msg), maxEstimateFailuresAdminNotificationLength)
	assert.True(utf8.ValidString(msg))
	assert.Contains(msg, "more (see logs)")
	assert.NotContains(msg, strings.Repeat("ž", maxEstimateErrorLengthInAdminNotification+1))
}
