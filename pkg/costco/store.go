package costco

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ordersDirName    = "orders"
	receiptsDirName  = "receipts"
	ordersFileName   = "orders.json"
	receiptsFileName = "receipts.json"
	manifestFileName = "manifest.json"

	// Downloaded history contains membership numbers, addresses and payment
	// descriptions, so everything is written user-only.
	historyDirMode  os.FileMode = 0700
	historyFileMode os.FileMode = 0600
)

// ManifestEntry is a one-line index record pointing at a downloaded file.
type ManifestEntry struct {
	ID          string  `json:"id"`
	Date        string  `json:"date"`
	Total       float64 `json:"total"`
	Description string  `json:"description,omitempty"`
	File        string  `json:"file"`
}

// Manifest summarises a download and indexes every file it produced.
type Manifest struct {
	LibraryVersion        string          `json:"library_version"`
	GeneratedAt           time.Time       `json:"generated_at"`
	Since                 string          `json:"since"`
	Until                 string          `json:"until"`
	OrderCount            int             `json:"order_count"`
	ReceiptCount          int             `json:"receipt_count"`
	ItemCount             int             `json:"item_count"`
	OnlineOrderTotal      float64         `json:"online_order_total"`
	WarehouseReceiptTotal float64         `json:"warehouse_receipt_total"`
	Warnings              []string        `json:"warnings,omitempty"`
	Orders                []ManifestEntry `json:"orders"`
	Receipts              []ManifestEntry `json:"receipts"`
}

// FileStore writes downloaded history to a directory as plain JSON:
//
//	<dir>/orders/<order number>.json   one file per online order
//	<dir>/receipts/<barcode>.json      one file per receipt, with line items
//	<dir>/orders.json                  every order in one array
//	<dir>/receipts.json                every receipt in one array
//	<dir>/manifest.json                summary and index of the above
//
// Because individual receipts are written as they arrive, an interrupted
// download can be resumed by pointing at the same directory again.
type FileStore struct {
	dir string
}

// NewFileStore prepares dir (and its subdirectories) to receive downloaded history.
func NewFileStore(dir string) (*FileStore, error) {
	for _, path := range []string{dir, filepath.Join(dir, ordersDirName), filepath.Join(dir, receiptsDirName)} {
		if err := os.MkdirAll(path, historyDirMode); err != nil {
			return nil, fmt.Errorf("creating %s: %w", path, err)
		}
	}
	return &FileStore{dir: dir}, nil
}

// Dir returns the root directory this store writes to.
func (s *FileStore) Dir() string { return s.dir }

// SaveOrder writes a single online order to <dir>/orders/<order number>.json.
func (s *FileStore) SaveOrder(order *OnlineOrder) error {
	id := order.OrderNumber
	if id == "" {
		id = order.OrderHeaderID
	}
	return writeJSONFile(s.orderPath(id), order)
}

// SaveReceipt writes a single receipt to <dir>/receipts/<barcode>.json.
func (s *FileStore) SaveReceipt(receipt *Receipt) error {
	return writeJSONFile(s.receiptPath(receipt.TransactionBarcode), receipt)
}

// HasReceipt reports whether a receipt has already been downloaded.
func (s *FileStore) HasReceipt(barcode string) bool {
	info, err := os.Stat(s.receiptPath(barcode))
	return err == nil && !info.IsDir()
}

// LoadReceipt reads a previously downloaded receipt back from disk.
func (s *FileStore) LoadReceipt(barcode string) (*Receipt, error) {
	data, err := os.ReadFile(s.receiptPath(barcode))
	if err != nil {
		return nil, err
	}

	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, fmt.Errorf("parsing stored receipt %s: %w", barcode, err)
	}
	return &receipt, nil
}

// WriteHistory writes every record plus the combined files and the manifest.
// It is safe to call after a partially resumed download; existing files are replaced.
func (s *FileStore) WriteHistory(history *History) error {
	manifest := Manifest{
		LibraryVersion:        Version,
		GeneratedAt:           history.GeneratedAt,
		Since:                 history.Since.Format("2006-01-02"),
		Until:                 history.Until.Format("2006-01-02"),
		OrderCount:            len(history.Orders),
		ReceiptCount:          len(history.Receipts),
		ItemCount:             history.ItemCount(),
		OnlineOrderTotal:      history.OnlineOrderTotal(),
		WarehouseReceiptTotal: history.WarehouseReceiptTotal(),
		Warnings:              history.Warnings,
		Orders:                make([]ManifestEntry, 0, len(history.Orders)),
		Receipts:              make([]ManifestEntry, 0, len(history.Receipts)),
	}

	for i := range history.Orders {
		order := history.Orders[i]
		if err := s.SaveOrder(&order); err != nil {
			return err
		}
		id := order.OrderNumber
		if id == "" {
			id = order.OrderHeaderID
		}
		manifest.Orders = append(manifest.Orders, ManifestEntry{
			ID:          id,
			Date:        dayOf(order.OrderPlacedDate),
			Total:       order.OrderTotal,
			Description: order.Status,
			File:        filepath.Join(ordersDirName, sanitizeFilename(id)+".json"),
		})
	}

	for i := range history.Receipts {
		receipt := history.Receipts[i]
		if err := s.SaveReceipt(&receipt); err != nil {
			return err
		}
		manifest.Receipts = append(manifest.Receipts, ManifestEntry{
			ID:          receipt.TransactionBarcode,
			Date:        dayOf(receipt.TransactionDateTime),
			Total:       receipt.Total,
			Description: receipt.WarehouseName,
			File:        filepath.Join(receiptsDirName, sanitizeFilename(receipt.TransactionBarcode)+".json"),
		})
	}

	if err := writeJSONFile(filepath.Join(s.dir, ordersFileName), history.Orders); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(s.dir, receiptsFileName), history.Receipts); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(s.dir, manifestFileName), manifest)
}

// ManifestPath returns the location of the manifest written by WriteHistory.
func (s *FileStore) ManifestPath() string {
	return filepath.Join(s.dir, manifestFileName)
}

func (s *FileStore) orderPath(id string) string {
	return filepath.Join(s.dir, ordersDirName, sanitizeFilename(id)+".json")
}

func (s *FileStore) receiptPath(barcode string) string {
	return filepath.Join(s.dir, receiptsDirName, sanitizeFilename(barcode)+".json")
}

func writeJSONFile(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, historyFileMode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// sanitizeFilename reduces an API identifier to characters that are safe on every
// filesystem, so a hostile or malformed identifier cannot escape the output directory.
func sanitizeFilename(id string) string {
	var builder strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}

	cleaned := builder.String()
	if cleaned == "" {
		return "unknown"
	}
	if len(cleaned) > 120 {
		cleaned = cleaned[:120]
	}
	return cleaned
}

// dayOf extracts the YYYY-MM-DD portion of an API timestamp.
func dayOf(timestamp string) string {
	if date, _, found := strings.Cut(timestamp, "T"); found {
		return date
	}
	return timestamp
}
