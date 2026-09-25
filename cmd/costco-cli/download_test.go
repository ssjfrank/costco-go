package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadConfig_DefaultsToADecadeEndingToday(t *testing.T) {
	now := time.Date(2025, 6, 15, 9, 30, 0, 0, time.UTC)

	options, err := downloadConfig{}.historyOptions(now)
	require.NoError(t, err)

	assert.Equal(t, "2015-06-15", options.Since.Format("2006-01-02"))
	assert.Equal(t, "2025-06-15", options.Until.Format("2006-01-02"))
}

func TestDownloadConfig_UsesExplicitDates(t *testing.T) {
	now := time.Date(2025, 6, 15, 9, 30, 0, 0, time.UTC)

	options, err := downloadConfig{Since: "2019-01-01", Until: "2020-02-29"}.historyOptions(now)
	require.NoError(t, err)

	assert.Equal(t, "2019-01-01", options.Since.Format("2006-01-02"))
	assert.Equal(t, "2020-02-29", options.Until.Format("2006-01-02"))
}

func TestDownloadConfig_RejectsUnparsableDates(t *testing.T) {
	now := time.Now()

	_, err := downloadConfig{Since: "01/02/2019"}.historyOptions(now)
	assert.ErrorContains(t, err, "-since")

	_, err = downloadConfig{Until: "yesterday"}.historyOptions(now)
	assert.ErrorContains(t, err, "-until")
}

func TestDownloadConfig_RejectsInvertedRange(t *testing.T) {
	now := time.Now()

	_, err := downloadConfig{Since: "2024-01-01", Until: "2023-01-01"}.historyOptions(now)
	assert.ErrorContains(t, err, "before")
}

func TestDownloadConfig_PassesTuningThrough(t *testing.T) {
	now := time.Now()

	options, err := downloadConfig{
		WindowDays:  90,
		PageSize:    25,
		Delay:       750 * time.Millisecond,
		Retries:     5,
		SkipDetails: true,
		Force:       true,
	}.historyOptions(now)
	require.NoError(t, err)

	assert.Equal(t, 90, options.WindowDays)
	assert.Equal(t, 25, options.PageSize)
	assert.Equal(t, 750*time.Millisecond, options.RequestDelay)
	assert.Equal(t, 5, options.MaxRetries)
	assert.True(t, options.SkipReceiptDetails)
	assert.True(t, options.Force)
}

func TestPrintSummary(t *testing.T) {
	history := &costco.History{
		Since: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Orders: []costco.OnlineOrder{
			{OrderNumber: "1", OrderTotal: 10},
			{OrderNumber: "2", OrderTotal: 15.50},
		},
		Receipts: []costco.Receipt{
			{TransactionBarcode: "BC1", Total: 100, ItemArray: []costco.ReceiptItem{{ItemNumber: "1"}}},
		},
		Warnings: []string{"receipt BC9 unavailable"},
	}

	var out bytes.Buffer
	printSummary(&out, history, "/tmp/costco-history")
	summary := out.String()

	assert.Contains(t, summary, "2 online order")
	assert.Contains(t, summary, "1 warehouse receipt")
	assert.Contains(t, summary, "1 receipt line item")
	assert.Contains(t, summary, "25.50")
	assert.Contains(t, summary, "100.00")
	assert.Contains(t, summary, "/tmp/costco-history")
	assert.Contains(t, summary, "receipt BC9 unavailable")
}

func TestPrintSummary_NoWarningsSectionWhenClean(t *testing.T) {
	var out bytes.Buffer
	printSummary(&out, &costco.History{}, "/tmp/costco-history")

	assert.NotContains(t, out.String(), "Warning")
}
