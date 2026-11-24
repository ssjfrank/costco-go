package costco

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileStore_CreatesPrivateDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "costco-history")

	store, err := NewFileStore(root)
	require.NoError(t, err)
	assert.Equal(t, root, store.Dir())

	for _, dir := range []string{root, filepath.Join(root, "orders"), filepath.Join(root, "receipts")} {
		info, statErr := os.Stat(dir)
		require.NoError(t, statErr, "expected %s to exist", dir)
		require.True(t, info.IsDir())

		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "%s should not be world readable", dir)
		}
	}
}

func TestFileStore_SaveAndLoadReceipt(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	receipt := &Receipt{
		TransactionBarcode:  "21134300501862509051323",
		TransactionDateTime: "2025-09-05T13:23:00",
		Total:               42.42,
		MembershipNumber:    "111222333",
		ItemArray: []ReceiptItem{
			{ItemNumber: "1553261", ItemDescription01: "GUAC BOWL", Unit: 1, Amount: 13.99},
		},
	}

	assert.False(t, store.HasReceipt(receipt.TransactionBarcode))
	require.NoError(t, store.SaveReceipt(receipt))
	assert.True(t, store.HasReceipt(receipt.TransactionBarcode))

	path := filepath.Join(store.Dir(), "receipts", "21134300501862509051323.json")
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "receipts contain personal data")
	}

	loaded, err := store.LoadReceipt(receipt.TransactionBarcode)
	require.NoError(t, err)
	assert.Equal(t, receipt.Total, loaded.Total)
	require.Len(t, loaded.ItemArray, 1)
	assert.Equal(t, "GUAC BOWL", loaded.ItemArray[0].ItemDescription01)
}

func TestFileStore_SaveOrder(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, store.SaveOrder(&OnlineOrder{
		OrderNumber:     "1234567",
		OrderPlacedDate: "2025-01-15",
		OrderTotal:      99.99,
	}))

	data, err := os.ReadFile(filepath.Join(store.Dir(), "orders", "1234567.json"))
	require.NoError(t, err)

	var saved OnlineOrder
	require.NoError(t, json.Unmarshal(data, &saved))
	assert.Equal(t, "1234567", saved.OrderNumber)
	assert.Equal(t, 99.99, saved.OrderTotal)
}

func TestFileStore_SanitizesIdentifiersUsedAsFilenames(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, store.SaveOrder(&OnlineOrder{OrderNumber: "../../escape/1", OrderTotal: 1}))

	entries, err := os.ReadDir(filepath.Join(store.Dir(), "orders"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "______escape_1.json", entries[0].Name())
}

func TestFileStore_LoadReceiptMissing(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.LoadReceipt("nope")
	assert.Error(t, err)
}

func TestFileStore_WriteHistoryProducesCombinedFilesAndManifest(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	history := &History{
		Since:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Until:       time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
		GeneratedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Orders: []OnlineOrder{
			{OrderNumber: "1234567", OrderPlacedDate: "2024-03-02", OrderTotal: 25.00},
		},
		Receipts: []Receipt{
			{
				TransactionBarcode:  "BC1",
				TransactionDateTime: "2024-04-05T10:00:00",
				WarehouseName:       "Issaquah",
				Total:               75.50,
				ItemArray:           []ReceiptItem{{ItemNumber: "1", Amount: 75.50, Unit: 1}},
			},
		},
		Warnings: []string{"receipt BC9 could not be downloaded"},
	}

	require.NoError(t, store.WriteHistory(history))

	var orders []OnlineOrder
	readJSON(t, filepath.Join(store.Dir(), "orders.json"), &orders)
	require.Len(t, orders, 1)
	assert.Equal(t, "1234567", orders[0].OrderNumber)

	var receipts []Receipt
	readJSON(t, filepath.Join(store.Dir(), "receipts.json"), &receipts)
	require.Len(t, receipts, 1)
	assert.Equal(t, "BC1", receipts[0].TransactionBarcode)

	var manifest Manifest
	readJSON(t, filepath.Join(store.Dir(), "manifest.json"), &manifest)
	assert.Equal(t, Version, manifest.LibraryVersion)
	assert.Equal(t, "2024-01-01", manifest.Since)
	assert.Equal(t, "2024-12-31", manifest.Until)
	assert.Equal(t, 1, manifest.OrderCount)
	assert.Equal(t, 1, manifest.ReceiptCount)
	assert.InDelta(t, 25.00, manifest.OnlineOrderTotal, 0.001)
	assert.InDelta(t, 75.50, manifest.WarehouseReceiptTotal, 0.001)
	assert.Equal(t, 1, manifest.ItemCount)
	require.Len(t, manifest.Warnings, 1)

	require.Len(t, manifest.Orders, 1)
	assert.Equal(t, "1234567", manifest.Orders[0].ID)
	assert.Equal(t, "2024-03-02", manifest.Orders[0].Date)
	assert.Equal(t, filepath.Join("orders", "1234567.json"), manifest.Orders[0].File)

	require.Len(t, manifest.Receipts, 1)
	assert.Equal(t, "BC1", manifest.Receipts[0].ID)
	assert.Equal(t, "2024-04-05", manifest.Receipts[0].Date)
	assert.Equal(t, "Issaquah", manifest.Receipts[0].Description)
	assert.Equal(t, filepath.Join("receipts", "BC1.json"), manifest.Receipts[0].File)
}

func TestFileStore_WriteHistoryIndexesTheWholeArchive(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// A full download, then a later run narrowed with -since.
	require.NoError(t, store.WriteHistory(&History{
		Since: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC),
		Orders: []OnlineOrder{
			{OrderNumber: "ORDER-2022", OrderPlacedDate: "2022-11-05", OrderTotal: 40.00},
		},
		Receipts: []Receipt{
			{TransactionBarcode: "BC2023", TransactionDateTime: "2023-07-04T11:30:00", Total: 60.00, MembershipNumber: "1",
				ItemArray: []ReceiptItem{{ItemNumber: "1", ItemDescription01: "MILK", Amount: 60.00, Unit: 1}}},
			{TransactionBarcode: "BC2022", TransactionDateTime: "2022-12-25T09:15:00", Total: 70.00, MembershipNumber: "1",
				ItemArray: []ReceiptItem{{ItemNumber: "2", ItemDescription01: "EGGS", Amount: 70.00, Unit: 1}}},
		},
	}))
	require.NoError(t, store.WriteHistory(&History{
		Since:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Until:       time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
		GeneratedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		Orders: []OnlineOrder{
			{OrderNumber: "ORDER-2024", OrderPlacedDate: "2024-06-15", OrderTotal: 100.00},
		},
		Receipts: []Receipt{
			{TransactionBarcode: "BC2024", TransactionDateTime: "2024-01-20T10:00:00", Total: 50.00, MembershipNumber: "1",
				ItemArray: []ReceiptItem{{ItemNumber: "3", ItemDescription01: "GUAC BOWL", Amount: 50.00, Unit: 1}}},
		},
	}))

	var orders []OnlineOrder
	readJSON(t, filepath.Join(store.Dir(), "orders.json"), &orders)
	require.Len(t, orders, 2, "orders from the earlier run stay in the combined file")
	assert.Equal(t, "ORDER-2024", orders[0].OrderNumber, "newest first")
	assert.Equal(t, "ORDER-2022", orders[1].OrderNumber)

	var receipts []Receipt
	readJSON(t, filepath.Join(store.Dir(), "receipts.json"), &receipts)
	require.Len(t, receipts, 3)
	assert.Equal(t, "BC2024", receipts[0].TransactionBarcode)
	assert.Equal(t, "BC2022", receipts[2].TransactionBarcode)

	var manifest Manifest
	readJSON(t, filepath.Join(store.Dir(), "manifest.json"), &manifest)
	assert.Equal(t, 2, manifest.OrderCount)
	assert.Equal(t, 3, manifest.ReceiptCount)
	assert.Equal(t, 3, manifest.ItemCount)
	assert.InDelta(t, 140.00, manifest.OnlineOrderTotal, 0.001)
	assert.InDelta(t, 180.00, manifest.WarehouseReceiptTotal, 0.001)
	assert.Equal(t, "2022-01-01", manifest.Since, "the range spans every run written here")
	assert.Equal(t, "2024-12-31", manifest.Until)
	assert.True(t, manifest.GeneratedAt.Equal(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)))

	require.Len(t, manifest.Receipts, 3)
	assert.Equal(t, "BC2022", manifest.Receipts[2].ID)
	assert.Equal(t, filepath.Join("receipts", "BC2022.json"), manifest.Receipts[2].File)
	require.Len(t, manifest.Orders, 2)
	assert.Equal(t, filepath.Join("orders", "ORDER-2022.json"), manifest.Orders[1].File)
}

func TestFileStore_WriteHistoryReportsFilesItCannotRead(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// What an interrupted write leaves behind.
	require.NoError(t, os.WriteFile(filepath.Join(store.Dir(), "receipts", "BROKEN.json"), []byte(`{"transactionBarcode": "BRO`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(store.Dir(), "receipts", "notes.txt"), []byte("mine"), 0600))

	require.NoError(t, store.WriteHistory(&History{
		Receipts: []Receipt{{TransactionBarcode: "BC1", TransactionDateTime: "2024-04-05T10:00:00", Total: 75.50}},
	}))

	var manifest Manifest
	readJSON(t, filepath.Join(store.Dir(), "manifest.json"), &manifest)
	assert.Equal(t, 1, manifest.ReceiptCount)
	require.Len(t, manifest.Warnings, 1, "a damaged file is reported, other people's files are ignored")
	assert.Contains(t, manifest.Warnings[0], "BROKEN.json")
}

func readJSON(t *testing.T, path string, target interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "expected %s to exist", path)
	require.NoError(t, json.Unmarshal(data, target))
}
