package provider

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestUSDTDoesNotEnableNonISOCurrencyInFiatProviders(t *testing.T) {
	currency, err := payment.NormalizePaymentCurrency(" usdt ")
	require.NoError(t, err)
	require.Equal(t, "USDT", currency)
	amount, err := payment.AmountToMinorUnit("12.34", currency)
	require.NoError(t, err)
	require.EqualValues(t, 1234, amount)
	_, err = payment.AmountToMinorUnit("12.340001", currency)
	require.Error(t, err)
	_, err = NewStripe("test", map[string]string{"secretKey": "test", "currency": "USDT"})
	require.Error(t, err)
	_, err = NewAirwallex("test", map[string]string{"clientId": "test", "apiKey": "test", "webhookSecret": "test", "apiBase": "https://api.airwallex.com", "currency": "USDT"})
	require.Error(t, err)
}
