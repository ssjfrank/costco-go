package costco

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	require.NoError(t, err)
	return parsed
}

func TestSplitDateRange_SingleDay(t *testing.T) {
	windows := SplitDateRange(mustDate(t, "2025-03-01"), mustDate(t, "2025-03-01"), 365)

	require.Len(t, windows, 1)
	assert.Equal(t, "2025-03-01", windows[0].StartDate())
	assert.Equal(t, "2025-03-01", windows[0].EndDate())
}

func TestSplitDateRange_NewestWindowFirst(t *testing.T) {
	windows := SplitDateRange(mustDate(t, "2025-01-01"), mustDate(t, "2025-01-10"), 5)

	require.Len(t, windows, 2)
	assert.Equal(t, "2025-01-06", windows[0].StartDate())
	assert.Equal(t, "2025-01-10", windows[0].EndDate())
	assert.Equal(t, "2025-01-01", windows[1].StartDate())
	assert.Equal(t, "2025-01-05", windows[1].EndDate())
}

func TestSplitDateRange_CoversRangeWithoutGapsOrOverlaps(t *testing.T) {
	since := mustDate(t, "2015-02-03")
	until := mustDate(t, "2025-08-17")

	windows := SplitDateRange(since, until, 90)

	require.NotEmpty(t, windows)
	assert.Equal(t, until.Format("2006-01-02"), windows[0].EndDate())
	assert.Equal(t, since.Format("2006-01-02"), windows[len(windows)-1].StartDate())

	for i, window := range windows {
		assert.False(t, window.End.Before(window.Start), "window %d ends before it starts", i)
		days := int(window.End.Sub(window.Start).Hours()/24) + 1
		assert.LessOrEqual(t, days, 90, "window %d spans more than the requested size", i)

		if i > 0 {
			previousStart := windows[i-1].Start
			assert.Equal(t, previousStart.AddDate(0, 0, -1).Format("2006-01-02"), window.EndDate(),
				"window %d is not adjacent to the previous window", i)
		}
	}
}

func TestSplitDateRange_SinceAfterUntil(t *testing.T) {
	windows := SplitDateRange(mustDate(t, "2025-06-01"), mustDate(t, "2025-01-01"), 30)

	assert.Empty(t, windows)
}

func TestSplitDateRange_NonPositiveWindowFallsBackToOneYear(t *testing.T) {
	windows := SplitDateRange(mustDate(t, "2023-01-01"), mustDate(t, "2023-12-31"), 0)

	require.Len(t, windows, 1)
	assert.Equal(t, "2023-01-01", windows[0].StartDate())
	assert.Equal(t, "2023-12-31", windows[0].EndDate())
}

func TestNormalizeReceiptDate(t *testing.T) {
	assert.Equal(t, "1/05/2025", normalizeReceiptDate("2025-01-05"))
	assert.Equal(t, "12/31/2024", normalizeReceiptDate("2024-12-31"))
	assert.Equal(t, "1/05/2025", normalizeReceiptDate("1/05/2025"), "already formatted values pass through")
	assert.Equal(t, "", normalizeReceiptDate(""))
}

// fakeCostcoAPI is an in-memory stand-in for Costco's GraphQL API. It filters its
// fixtures by the requested date range so tests can verify that the downloader
// walks the entire history rather than a single window.
type fakeCostcoAPI struct {
	mu sync.Mutex

	orders   []OnlineOrder
	receipts []Receipt

	orderRequests    []map[string]interface{}
	receiptRequests  []map[string]interface{}
	detailBarcodes   []string
	failDetailFor    map[string]bool
	failReceiptLists bool
}

func newFakeCostcoAPI() *fakeCostcoAPI {
	return &fakeCostcoAPI{
		orders: []OnlineOrder{
			{OrderNumber: "ORDER-2024-A", OrderPlacedDate: "2024-06-15", OrderTotal: 101.10},
			{OrderNumber: "ORDER-2024-B", OrderPlacedDate: "2024-02-01", OrderTotal: 202.20},
			{OrderNumber: "ORDER-2023-A", OrderPlacedDate: "2023-03-10", OrderTotal: 303.30},
			{OrderNumber: "ORDER-2022-A", OrderPlacedDate: "2022-11-05", OrderTotal: 404.40},
		},
		receipts: []Receipt{
			{TransactionBarcode: "BC2024", TransactionDateTime: "2024-01-20T10:00:00", ReceiptType: "Warehouse", DocumentType: "warehouse", Total: 51.51, WarehouseName: "Issaquah"},
			{TransactionBarcode: "BC2023", TransactionDateTime: "2023-07-04T11:30:00", ReceiptType: "Gas Station", DocumentType: "fuel", Total: 62.62, WarehouseName: "Issaquah"},
			{TransactionBarcode: "BC2022", TransactionDateTime: "2022-12-25T09:15:00", ReceiptType: "Warehouse", DocumentType: "warehouse", Total: 73.73, WarehouseName: "Kirkland"},
		},
		failDetailFor: map[string]bool{},
	}
}

func (f *fakeCostcoAPI) start(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(server.Close)
	return server
}

func (f *fakeCostcoAPI) handle(w http.ResponseWriter, r *http.Request) {
	var req GraphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch {
	case strings.Contains(req.Query, "getOnlineOrders"):
		f.handleOrders(w, req)
	case req.Variables["barcode"] != nil:
		f.handleReceiptDetail(w, req)
	default:
		f.handleReceiptList(w, req)
	}
}

func (f *fakeCostcoAPI) handleOrders(w http.ResponseWriter, req GraphQLRequest) {
	f.mu.Lock()
	f.orderRequests = append(f.orderRequests, req.Variables)
	f.mu.Unlock()

	start, err := time.Parse("2006-01-02", req.Variables["startDate"].(string))
	if err != nil {
		writeGraphQLError(w, fmt.Sprintf("bad order startDate: %v", err))
		return
	}
	end, err := time.Parse("2006-01-02", req.Variables["endDate"].(string))
	if err != nil {
		writeGraphQLError(w, fmt.Sprintf("bad order endDate: %v", err))
		return
	}

	var matched []OnlineOrder
	for _, order := range f.orders {
		placed, parseErr := time.Parse("2006-01-02", order.OrderPlacedDate)
		if parseErr != nil {
			continue
		}
		if !placed.Before(start) && !placed.After(end) {
			matched = append(matched, order)
		}
	}

	pageNumber := int(req.Variables["pageNumber"].(float64))
	pageSize := int(req.Variables["pageSize"].(float64))
	from := (pageNumber - 1) * pageSize
	to := from + pageSize
	if from > len(matched) {
		from = len(matched)
	}
	if to > len(matched) {
		to = len(matched)
	}

	writeGraphQLData(w, map[string]interface{}{
		"getOnlineOrders": []OnlineOrdersResponse{{
			PageNumber:           pageNumber,
			PageSize:             pageSize,
			TotalNumberOfRecords: len(matched),
			BCOrders:             matched[from:to],
		}},
	})
}

func (f *fakeCostcoAPI) handleReceiptList(w http.ResponseWriter, req GraphQLRequest) {
	f.mu.Lock()
	f.receiptRequests = append(f.receiptRequests, req.Variables)
	shouldFail := f.failReceiptLists
	f.mu.Unlock()

	if shouldFail {
		writeGraphQLError(w, "receipt lookup unavailable")
		return
	}

	// Costco's receipt endpoint only understands M/DD/YYYY, so reject anything else.
	start, err := time.Parse("1/02/2006", req.Variables["startDate"].(string))
	if err != nil {
		writeGraphQLError(w, fmt.Sprintf("bad receipt startDate: %v", err))
		return
	}
	end, err := time.Parse("1/02/2006", req.Variables["endDate"].(string))
	if err != nil {
		writeGraphQLError(w, fmt.Sprintf("bad receipt endDate: %v", err))
		return
	}

	response := ReceiptsWithCountsResponse{}
	for _, receipt := range f.receipts {
		stamp, parseErr := time.Parse("2006-01-02T15:04:05", receipt.TransactionDateTime)
		if parseErr != nil {
			continue
		}
		day := time.Date(stamp.Year(), stamp.Month(), stamp.Day(), 0, 0, 0, 0, time.UTC)
		if day.Before(start) || day.After(end) {
			continue
		}
		summary := Receipt{
			TransactionBarcode:  receipt.TransactionBarcode,
			TransactionDateTime: receipt.TransactionDateTime,
			ReceiptType:         receipt.ReceiptType,
			DocumentType:        receipt.DocumentType,
			Total:               receipt.Total,
			WarehouseName:       receipt.WarehouseName,
		}
		response.Receipts = append(response.Receipts, summary)
		if receipt.DocumentType == "fuel" {
			response.GasStation++
		} else {
			response.InWarehouse++
		}
	}

	writeGraphQLData(w, map[string]interface{}{"receiptsWithCounts": response})
}

func (f *fakeCostcoAPI) handleReceiptDetail(w http.ResponseWriter, req GraphQLRequest) {
	barcode := req.Variables["barcode"].(string)

	f.mu.Lock()
	f.detailBarcodes = append(f.detailBarcodes, barcode)
	shouldFail := f.failDetailFor[barcode]
	f.mu.Unlock()

	if shouldFail {
		writeGraphQLError(w, "receipt detail unavailable")
		return
	}

	for _, receipt := range f.receipts {
		if receipt.TransactionBarcode != barcode {
			continue
		}
		detailed := receipt
		detailed.SubTotal = receipt.Total
		detailed.MembershipNumber = "111222333"
		detailed.ItemArray = []ReceiptItem{
			{ItemNumber: "1553261", ItemDescription01: "GUAC BOWL", Unit: 1, Amount: receipt.Total},
		}
		writeGraphQLData(w, map[string]interface{}{
			"receiptsWithCounts": map[string]interface{}{"receipts": []Receipt{detailed}},
		})
		return
	}

	writeGraphQLData(w, map[string]interface{}{
		"receiptsWithCounts": map[string]interface{}{"receipts": []Receipt{}},
	})
}

func (f *fakeCostcoAPI) detailCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.detailBarcodes)
}

func writeGraphQLData(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func writeGraphQLError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []map[string]string{{"message": message}},
	})
}

func newHistoryTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	return seedValidToken(&Client{
		httpClient: &http.Client{Transport: &testTransport{baseURL: serverURL}},
		config: Config{
			Email:              "test@example.com",
			WarehouseNumber:    "847",
			TokenRefreshBuffer: 5 * time.Minute,
		},
	})
}

func TestFetchAllOnlineOrders_PaginatesUntilComplete(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	orders, err := client.FetchAllOnlineOrders(context.Background(), "2022-01-01", "2024-12-31", 2)
	require.NoError(t, err)

	require.Len(t, orders, 4)
	assert.Equal(t, "ORDER-2024-A", orders[0].OrderNumber)
	assert.Equal(t, "ORDER-2022-A", orders[3].OrderNumber)
	assert.Len(t, api.orderRequests, 2, "expected one request per page")
}

func TestDownloadHistory_DownloadsEveryOrderAndReceipt(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	history, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
		PageSize:   2,
	})
	require.NoError(t, err)

	assert.Len(t, history.Orders, 4)
	assert.Len(t, history.Receipts, 3)
	assert.Empty(t, history.Warnings)

	// Receipts must arrive fully detailed, not as list summaries.
	for _, receipt := range history.Receipts {
		assert.NotEmpty(t, receipt.ItemArray, "receipt %s is missing line items", receipt.TransactionBarcode)
		assert.Equal(t, "111222333", receipt.MembershipNumber)
	}

	// Newest first keeps the most useful data at the top of the output.
	assert.Equal(t, "BC2024", history.Receipts[0].TransactionBarcode)
	assert.Equal(t, "BC2022", history.Receipts[2].TransactionBarcode)
}

func TestDownloadHistory_QueriesReceiptsWithCostcoDateFormat(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	_, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2023-01-01"),
		Until:      mustDate(t, "2023-12-31"),
		WindowDays: 365,
	})
	require.NoError(t, err)

	require.NotEmpty(t, api.receiptRequests)
	assert.Equal(t, "1/01/2023", api.receiptRequests[0]["startDate"])
	assert.Equal(t, "12/31/2023", api.receiptRequests[0]["endDate"])
}

func TestDownloadHistory_DeduplicatesAcrossWindows(t *testing.T) {
	api := newFakeCostcoAPI()
	// A short window size means many API calls; boundaries must not duplicate records.
	client := newHistoryTestClient(t, api.start(t).URL)

	history, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 30,
	})
	require.NoError(t, err)

	assert.Len(t, history.Orders, 4)
	assert.Len(t, history.Receipts, 3)
}

func TestDownloadHistory_SkipReceiptDetails(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	history, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:              mustDate(t, "2022-01-01"),
		Until:              mustDate(t, "2024-12-31"),
		WindowDays:         365,
		SkipReceiptDetails: true,
	})
	require.NoError(t, err)

	assert.Len(t, history.Receipts, 3)
	assert.Zero(t, api.detailCallCount(), "detail lookups should be skipped")
}

func TestDownloadHistory_RecordsWarningWhenReceiptDetailFails(t *testing.T) {
	api := newFakeCostcoAPI()
	api.failDetailFor["BC2023"] = true
	client := newHistoryTestClient(t, api.start(t).URL)

	history, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
	})
	require.NoError(t, err, "one bad receipt must not abort a multi-year download")

	assert.Len(t, history.Receipts, 3, "the list summary is kept when the detail lookup fails")
	require.Len(t, history.Warnings, 1)
	assert.Contains(t, history.Warnings[0], "BC2023")
}

func TestDownloadHistory_FailsWhenFirstWindowFails(t *testing.T) {
	api := newFakeCostcoAPI()
	api.failReceiptLists = true
	client := newHistoryTestClient(t, api.start(t).URL)

	_, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
	})

	require.Error(t, err, "an immediate failure usually means expired tokens, so stop early")
	assert.Contains(t, err.Error(), "receipt")
}

func TestDownloadHistory_ReportsProgress(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	var messages []string
	_, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2024-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
		Progress:   func(message string) { messages = append(messages, message) },
	})
	require.NoError(t, err)

	assert.NotEmpty(t, messages)
	assert.Contains(t, strings.Join(messages, "\n"), "2024")
}

func TestDownloadHistory_ResumesFromStore(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	options := HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
		Store:      store,
	}

	first, err := client.DownloadHistory(context.Background(), options)
	require.NoError(t, err)
	require.Len(t, first.Receipts, 3)
	firstDetailCalls := api.detailCallCount()
	assert.Equal(t, 3, firstDetailCalls)

	second, err := client.DownloadHistory(context.Background(), options)
	require.NoError(t, err)

	assert.Len(t, second.Receipts, 3)
	assert.Equal(t, firstDetailCalls, api.detailCallCount(), "already-saved receipts should be read from disk")
	assert.NotEmpty(t, second.Receipts[0].ItemArray, "resumed receipts keep their line items")
}

func TestDownloadHistory_ForceRefetchIgnoresStoredReceipts(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	options := HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
		Store:      store,
		Force:      true,
	}

	_, err = client.DownloadHistory(context.Background(), options)
	require.NoError(t, err)
	_, err = client.DownloadHistory(context.Background(), options)
	require.NoError(t, err)

	assert.Equal(t, 6, api.detailCallCount())
}

// TestDownloadHistory_WritesCompleteOutputTree exercises the whole path a CLI user
// takes: download everything, then write it out for offline use.
func TestDownloadHistory_WritesCompleteOutputTree(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	history, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since:      mustDate(t, "2022-01-01"),
		Until:      mustDate(t, "2024-12-31"),
		WindowDays: 365,
		Store:      store,
	})
	require.NoError(t, err)
	require.NoError(t, store.WriteHistory(history))

	for _, relative := range []string{
		"manifest.json",
		"orders.json",
		"receipts.json",
		filepath.Join("orders", "ORDER-2024-A.json"),
		filepath.Join("receipts", "BC2024.json"),
		filepath.Join("receipts", "BC2022.json"),
	} {
		_, statErr := os.Stat(filepath.Join(dir, relative))
		assert.NoError(t, statErr, "expected %s to be written", relative)
	}

	var manifest Manifest
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &manifest))

	assert.Equal(t, 4, manifest.OrderCount)
	assert.Equal(t, 3, manifest.ReceiptCount)
	assert.Equal(t, 3, manifest.ItemCount)
	assert.Equal(t, "2022-01-01", manifest.Since)
	assert.Equal(t, "2024-12-31", manifest.Until)
	assert.Empty(t, manifest.Warnings)
}

func TestDownloadHistory_RejectsFutureSince(t *testing.T) {
	api := newFakeCostcoAPI()
	client := newHistoryTestClient(t, api.start(t).URL)

	_, err := client.DownloadHistory(context.Background(), HistoryOptions{
		Since: mustDate(t, "2025-01-01"),
		Until: mustDate(t, "2024-01-01"),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "after")
}
