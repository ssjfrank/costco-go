package costco

import "context"

var _ CostcoClient = (*Client)(nil)

// CostcoClient defines the interface for interacting with Costco's API.
// This interface can be used for mocking in tests or creating alternative implementations.
//
// Example usage with mocking:
//
//	type MockClient struct {
//	    mock.Mock
//	}
//
//	func (m *MockClient) GetOnlineOrders(ctx context.Context, startDate, endDate string, pageNumber, pageSize int) (*OnlineOrdersResponse, error) {
//	    args := m.Called(ctx, startDate, endDate, pageNumber, pageSize)
//	    return args.Get(0).(*OnlineOrdersResponse), args.Error(1)
//	}
//
//	// Use in tests
//	mockClient := new(MockClient)
//	mockClient.On("GetOnlineOrders", ...).Return(&OnlineOrdersResponse{...}, nil)
type CostcoClient interface {
	// DownloadHistory downloads every online order and warehouse receipt in the
	// configured date range, expanding each receipt into its full line items.
	DownloadHistory(ctx context.Context, opts HistoryOptions) (*History, error)

	// FetchAllOnlineOrders pages through every online order between two
	// YYYY-MM-DD dates.
	FetchAllOnlineOrders(ctx context.Context, startDate, endDate string, pageSize int) ([]OnlineOrder, error)

	// GetOnlineOrders retrieves a single page of online orders from Costco.com.
	GetOnlineOrders(ctx context.Context, startDate, endDate string, pageNumber, pageSize int) (*OnlineOrdersResponse, error)

	// GetReceipts retrieves warehouse receipt summaries within the specified date range.
	// Can filter by documentType ("all", "warehouse", "fuel") and documentSubType.
	GetReceipts(ctx context.Context, startDate, endDate, documentType, documentSubType string) (*ReceiptsWithCountsResponse, error)

	// GetReceiptDetail retrieves full details for a specific receipt identified by barcode.
	// documentType should be "warehouse" or "fuel" depending on the receipt type.
	GetReceiptDetail(ctx context.Context, barcode, documentType string) (*Receipt, error)
}
