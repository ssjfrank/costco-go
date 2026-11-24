package costco

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// Manifest summarises the archive directory and indexes every record in it,
// including records saved by earlier runs. Since and Until span every run
// written to the directory; GeneratedAt and Warnings describe the latest one.
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

// WriteHistory saves every record of this run, then rewrites the combined files
// and the manifest from everything in the directory. Records saved by earlier
// runs stay indexed, so a later run over a narrower date range adds to the
// archive instead of shrinking its index.
func (s *FileStore) WriteHistory(history *History) error {
	for i := range history.Orders {
		if err := s.SaveOrder(&history.Orders[i]); err != nil {
			return err
		}
	}
	for i := range history.Receipts {
		if err := s.SaveReceipt(&history.Receipts[i]); err != nil {
			return err
		}
	}

	orders, unreadableOrders, err := readArchive[OnlineOrder](s.dir, ordersDirName)
	if err != nil {
		return err
	}
	receipts, unreadableReceipts, err := readArchive[Receipt](s.dir, receiptsDirName)
	if err != nil {
		return err
	}
	sort.SliceStable(orders, func(i, j int) bool {
		return orders[i].record.OrderPlacedDate > orders[j].record.OrderPlacedDate
	})
	sort.SliceStable(receipts, func(i, j int) bool {
		return receipts[i].record.TransactionDateTime > receipts[j].record.TransactionDateTime
	})

	archive := History{
		Orders:   make([]OnlineOrder, 0, len(orders)),
		Receipts: make([]Receipt, 0, len(receipts)),
	}
	orderEntries := make([]ManifestEntry, 0, len(orders))
	for _, stored := range orders {
		order := stored.record
		id := order.OrderNumber
		if id == "" {
			id = order.OrderHeaderID
		}
		archive.Orders = append(archive.Orders, order)
		orderEntries = append(orderEntries, ManifestEntry{
			ID:          id,
			Date:        dayOf(order.OrderPlacedDate),
			Total:       order.OrderTotal,
			Description: order.Status,
			File:        stored.file,
		})
	}
	receiptEntries := make([]ManifestEntry, 0, len(receipts))
	for _, stored := range receipts {
		receipt := stored.record
		archive.Receipts = append(archive.Receipts, receipt)
		receiptEntries = append(receiptEntries, ManifestEntry{
			ID:          receipt.TransactionBarcode,
			Date:        dayOf(receipt.TransactionDateTime),
			Total:       receipt.Total,
			Description: receipt.WarehouseName,
			File:        stored.file,
		})
	}

	warnings := append([]string(nil), history.Warnings...)
	for _, file := range append(unreadableOrders, unreadableReceipts...) {
		warnings = append(warnings, fmt.Sprintf("%s could not be read and is left out of the index", file))
	}

	since, until := s.archiveRange(history)
	manifest := Manifest{
		LibraryVersion:        Version,
		GeneratedAt:           history.GeneratedAt,
		Since:                 since.Format("2006-01-02"),
		Until:                 until.Format("2006-01-02"),
		OrderCount:            len(archive.Orders),
		ReceiptCount:          len(archive.Receipts),
		ItemCount:             archive.ItemCount(),
		OnlineOrderTotal:      archive.OnlineOrderTotal(),
		WarehouseReceiptTotal: archive.WarehouseReceiptTotal(),
		Warnings:              warnings,
		Orders:                orderEntries,
		Receipts:              receiptEntries,
	}

	if err := writeJSONFile(filepath.Join(s.dir, ordersFileName), archive.Orders); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(s.dir, receiptsFileName), archive.Receipts); err != nil {
		return err
	}
	return writeJSONFile(filepath.Join(s.dir, manifestFileName), manifest)
}

// archiveRange widens this run's date range to cover the ranges of earlier
// runs, as recorded in the manifest they left behind.
func (s *FileStore) archiveRange(history *History) (since, until time.Time) {
	since, until = history.Since, history.Until

	data, err := os.ReadFile(s.ManifestPath())
	if err != nil {
		return since, until
	}
	var previous Manifest
	if json.Unmarshal(data, &previous) != nil {
		return since, until
	}
	if earlier, err := time.Parse("2006-01-02", previous.Since); err == nil && earlier.Before(since) {
		since = earlier
	}
	if later, err := time.Parse("2006-01-02", previous.Until); err == nil && later.After(until) {
		until = later
	}
	return since, until
}

// archived is a record read back from the store, with the file it lives in
// relative to the store's root.
type archived[T any] struct {
	record T
	file   string
}

// readArchive decodes every JSON file in a subdirectory of the store. A file
// that cannot be decoded is returned by name rather than failing the call: one
// damaged file must not keep the rest of the archive out of the index.
func readArchive[T any](root, subdir string) (records []archived[T], unreadable []string, err error) {
	entries, err := os.ReadDir(filepath.Join(root, subdir))
	if err != nil {
		return nil, nil, fmt.Errorf("listing %s: %w", filepath.Join(root, subdir), err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		file := filepath.Join(subdir, entry.Name())

		var record T
		data, readErr := os.ReadFile(filepath.Join(root, file))
		if readErr != nil || json.Unmarshal(data, &record) != nil {
			unreadable = append(unreadable, file)
			continue
		}
		records = append(records, archived[T]{record: record, file: file})
	}
	return records, unreadable, nil
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
