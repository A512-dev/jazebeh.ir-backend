package businessflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

const (
	maxBundleActionFileBytes             int64 = 50 << 20
	maxBundleActionFileUncompressedBytes int64 = 256 << 20
	// Excelize spills worksheet XML over this limit to its temporary directory,
	// avoiding a full worksheet allocation in the scheduler process.
	maxBundleActionFileXMLInMemoryBytes int64 = 8 << 20
	// The parser must retain unique UIDs until they are persisted, so bound row
	// count as well as compressed and uncompressed workbook sizes.
	maxBundleActionFileRows int64 = 500_000
)

const (
	defaultBundleActionFileStorageRoot = "data/uploads/bundle-action-files"
	bundleActionFileStagingDirectory   = ".bundle-action-file-staging"
)

// BundleActionFlow defines bundle action-file management and reporting operations.
type BundleActionFlow interface {
	Upload(context.Context, uint, uint, string, string, io.Reader) (*models.BundleActionFile, error)
	Get(context.Context, uint, uint, int64) (*models.BundleActionFile, error)
	List(context.Context, uint, uint, int, int) ([]*models.BundleActionFile, int64, error)
	Delete(context.Context, uint, uint, int64) error
	Summary(context.Context, uint, uint) (*models.BundleActionSummary, error)
	TagMetrics(context.Context, uint, uint) ([]*models.BundleActionTagMetric, error)
	CampaignMetric(context.Context, uint, string) (*models.BundleActionCampaignMetric, error)
	ExecuteBundleActionFile(context.Context, int64, time.Time) error
}

// BundleActionFlowImpl coordinates bundle action-file storage, processing, and reporting.
type BundleActionFlowImpl struct {
	actionRepository   repository.BundleActionRepository
	bundleRepository   repository.BundleRepository
	campaignRepository repository.CampaignRepository
	db                 *gorm.DB
	storageRoot        string
}

// NewBundleActionFlow creates a flow for managing bundle action files.
func NewBundleActionFlow(
	actionRepository repository.BundleActionRepository,
	bundleRepository repository.BundleRepository,
	campaignRepository repository.CampaignRepository,
	db *gorm.DB,
) *BundleActionFlowImpl {
	storageRoot := strings.TrimSpace(os.Getenv("BUNDLE_ACTION_FILE_STORAGE_ROOT"))
	if storageRoot == "" {
		storageRoot = defaultBundleActionFileStorageRoot
	}
	return &BundleActionFlowImpl{
		actionRepository:   actionRepository,
		bundleRepository:   bundleRepository,
		campaignRepository: campaignRepository,
		db:                 db,
		storageRoot:        storageRoot,
	}
}

// getCustomerBundle returns a bundle after confirming that it belongs to the customer.
func (flow *BundleActionFlowImpl) getCustomerBundle(ctx context.Context, customerID, bundleID uint) (*models.Bundle, error) {
	bundle, err := flow.bundleRepository.ByID(ctx, bundleID)
	if err != nil {
		return nil, err
	}
	if bundle.CustomerID != customerID {
		return nil, ErrBundleAccessDenied
	}
	return bundle, nil
}

// actionFileStoragePath returns the destination path for a bundle action file.
func actionFileStoragePath(storageRoot string, bundleID uint, actionFileID int64) string {
	return filepath.Join(storageRoot, fmt.Sprintf("%d", bundleID), fmt.Sprintf("%d.xlsx", actionFileID))
}

func actionFileStagingPath(storageRoot string) (string, error) {
	stagingDir := filepath.Join(storageRoot, bundleActionFileStagingDirectory)
	if err := os.MkdirAll(stagingDir, 0750); err != nil {
		return "", fmt.Errorf("create action staging directory: %w", err)
	}
	staged, err := os.CreateTemp(stagingDir, "bundle-action-*.xlsx")
	if err != nil {
		return "", fmt.Errorf("create action staging file: %w", err)
	}
	stagedPath := staged.Name()
	keep := false
	defer func() {
		if !keep {
			_ = staged.Close()
			_ = os.Remove(stagedPath)
		}
	}()
	if err := staged.Chmod(0640); err != nil {
		return "", fmt.Errorf("set action staging permissions: %w", err)
	}
	if err := staged.Close(); err != nil {
		return "", fmt.Errorf("close action staging file: %w", err)
	}
	keep = true
	return stagedPath, nil
}

func writeStagedActionFile(stagedPath string, fileData []byte) error {
	staged, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("open action staging file: %w", err)
	}
	defer staged.Close()
	if _, err := staged.Write(fileData); err != nil {
		return fmt.Errorf("write action staging file: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return fmt.Errorf("sync action staging file: %w", err)
	}
	return nil
}

func publishStagedActionFile(stagedPath, storagePath string) error {
	storageDir := filepath.Dir(storagePath)
	if err := os.MkdirAll(storageDir, 0750); err != nil {
		return fmt.Errorf("create action storage: %w", err)
	}
	if err := os.Rename(stagedPath, storagePath); err != nil {
		return fmt.Errorf("publish action storage: %w", err)
	}
	// The staging directory is below the same storage root, so rename is atomic.
	// Syncing the destination directory makes the rename durable before the row
	// becomes claimable.
	dir, err := os.Open(storageDir)
	if err != nil {
		return fmt.Errorf("open action storage directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync action storage directory: %w", err)
	}
	return nil
}

// Upload validates, stores, and queues an XLSX action file for a customer bundle.
func (flow *BundleActionFlowImpl) Upload(
	ctx context.Context,
	customerID, bundleID uint,
	fileName, actionLevel string,
	source io.Reader,
) (*models.BundleActionFile, error) {
	if _, err := flow.getCustomerBundle(ctx, customerID, bundleID); err != nil {
		return nil, err
	}
	if strings.ToLower(filepath.Ext(fileName)) != ".xlsx" {
		return nil, NewBusinessError("ACTION_FILE_FORMAT_INVALID", "only .xlsx action files are supported", nil)
	}
	fileData, err := io.ReadAll(io.LimitReader(source, maxBundleActionFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(fileData)) > maxBundleActionFileBytes {
		return nil, NewBusinessError("ACTION_FILE_TOO_LARGE", "action file exceeds the 50 MB limit", nil)
	}
	if len(fileData) == 0 {
		return nil, NewBusinessError("ACTION_FILE_EMPTY", "action file is empty", nil)
	}
	stagedPath, err := actionFileStagingPath(flow.storageRoot)
	if err != nil {
		return nil, err
	}
	if err := writeStagedActionFile(stagedPath, fileData); err != nil {
		_ = os.Remove(stagedPath)
		return nil, err
	}

	fileHash := sha256.Sum256(fileData)
	now := utils.UTCNow()
	actionFile := &models.BundleActionFile{
		BundleID:             bundleID,
		OriginalFileName:     filepath.Base(fileName),
		ActionLevel:          actionLevel,
		ContentSHA256:        hex.EncodeToString(fileHash[:]),
		Status:               models.BundleActionFileStaging,
		UploadedByCustomerID: customerID,
		CreatedAt:            now,
		UpdatedAt:            now,
		StoragePath:          stagedPath,
	}
	if err := flow.actionRepository.Create(ctx, actionFile); err != nil {
		_ = os.Remove(stagedPath)
		return nil, err
	}
	storagePath := actionFileStoragePath(flow.storageRoot, bundleID, actionFile.ID)
	if err := publishStagedActionFile(stagedPath, storagePath); err != nil {
		flow.cleanupStagedActionFile(actionFile.ID, stagedPath, storagePath)
		return nil, err
	}
	if err := flow.actionRepository.PromotePending(ctx, actionFile.ID, storagePath, utils.UTCNow()); err != nil {
		flow.cleanupStagedActionFile(actionFile.ID, stagedPath, storagePath)
		return nil, fmt.Errorf("publish action file row: %w", err)
	}
	actionFile.Status = models.BundleActionFilePending
	actionFile.StoragePath = storagePath
	return actionFile, nil
}

// cleanupStagedActionFile removes bytes only after its non-claimable row was
// removed. If promotion might have succeeded, retaining the final file is safer
// than leaving a pending row without storage.
func (flow *BundleActionFlowImpl) cleanupStagedActionFile(actionFileID int64, stagedPath, storagePath string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	discarded, err := flow.actionRepository.DiscardStaged(cleanupCtx, actionFileID)
	if err != nil || !discarded {
		return
	}
	_ = os.Remove(stagedPath)
	_ = os.Remove(storagePath)
}

// Get returns an action file when it belongs to the customer's bundle.
func (flow *BundleActionFlowImpl) Get(
	ctx context.Context,
	customerID, bundleID uint,
	actionFileID int64,
) (*models.BundleActionFile, error) {
	if _, err := flow.getCustomerBundle(ctx, customerID, bundleID); err != nil {
		return nil, err
	}
	actionFile, err := flow.actionRepository.ByID(ctx, actionFileID)
	if err == nil && actionFile.BundleID != bundleID {
		return nil, ErrBundleAccessDenied
	}
	return actionFile, err
}

// List returns a page of action files for a customer's bundle.
func (flow *BundleActionFlowImpl) List(
	ctx context.Context,
	customerID, bundleID uint,
	page, limit int,
) ([]*models.BundleActionFile, int64, error) {
	if _, err := flow.getCustomerBundle(ctx, customerID, bundleID); err != nil {
		return nil, 0, err
	}
	return flow.actionRepository.List(ctx, bundleID, limit, (page-1)*limit)
}

// Delete requests deletion of a processed action file owned by the customer's bundle.
func (flow *BundleActionFlowImpl) Delete(ctx context.Context, customerID, bundleID uint, actionFileID int64) error {
	if _, err := flow.getCustomerBundle(ctx, customerID, bundleID); err != nil {
		return err
	}
	actionFile, err := flow.actionRepository.ByID(ctx, actionFileID)
	if err != nil {
		return err
	}
	if actionFile.BundleID != bundleID {
		return ErrBundleAccessDenied
	}
	if actionFile.Status != models.BundleActionFileProcessed {
		return NewBusinessError("ACTION_FILE_NOT_DELETABLE", "only processed action files can be deleted", nil)
	}
	return flow.actionRepository.RequestDelete(ctx, actionFileID, customerID, utils.UTCNow())
}

// Summary returns aggregate action metrics for a customer's bundle.
func (flow *BundleActionFlowImpl) Summary(ctx context.Context, customerID, bundleID uint) (*models.BundleActionSummary, error) {
	if _, err := flow.getCustomerBundle(ctx, customerID, bundleID); err != nil {
		return nil, err
	}
	return flow.actionRepository.Summary(ctx, bundleID)
}

// TagMetrics returns action metrics grouped by tag for a customer's bundle.
func (flow *BundleActionFlowImpl) TagMetrics(ctx context.Context, customerID, bundleID uint) ([]*models.BundleActionTagMetric, error) {
	if _, err := flow.getCustomerBundle(ctx, customerID, bundleID); err != nil {
		return nil, err
	}
	return flow.actionRepository.TagMetrics(ctx, bundleID)
}

// CampaignMetric returns action metrics for a campaign, if the campaign belongs to a bundle.
func (flow *BundleActionFlowImpl) CampaignMetric(
	ctx context.Context,
	customerID uint,
	campaignUUID string,
) (*models.BundleActionCampaignMetric, error) {
	campaign, err := getCampaign(ctx, flow.campaignRepository, campaignUUID, customerID)
	if err != nil {
		return nil, err
	}
	if campaign.BundleID == nil {
		return &models.BundleActionCampaignMetric{CampaignID: campaign.ID}, nil
	}
	return flow.actionRepository.CampaignMetric(ctx, *campaign.BundleID, campaign.ID)
}

// ExecuteBundleActionFile processes a claimed action file or completes its pending deletion.
// Every durable mutation is fenced by leaseStartedAt so a reclaimed worker cannot
// publish after a newer worker has claimed the file.
func (flow *BundleActionFlowImpl) ExecuteBundleActionFile(ctx context.Context, actionFileID int64, leaseStartedAt time.Time) error {
	actionFile, err := flow.actionRepository.ByID(ctx, actionFileID)
	if err != nil {
		return err
	}
	if actionFile.StartedAt == nil || !actionFile.StartedAt.Equal(leaseStartedAt) {
		return repository.ErrBundleActionFileLeaseLost
	}
	if actionFile.Status == models.BundleActionFileDeletePending {
		return flow.actionRepository.Complete(ctx, actionFileID, leaseStartedAt, utils.UTCNow())
	}
	if actionFile.Status != models.BundleActionFileProcessing {
		return nil
	}
	uids, totalRowCount, duplicateUIDCount, invalidUIDCount, err := parseBundleActionXLSX(actionFile.StoragePath)
	if err != nil {
		_ = flow.actionRepository.Fail(
			context.Background(),
			actionFileID,
			leaseStartedAt,
			utils.UTCNow(),
			"ACTION_FILE_PARSE_FAILED",
			"The action file could not be parsed",
		)
		return err
	}
	now := utils.UTCNow()
	if err := flow.actionRepository.StoreUIDsAndResult(ctx, actionFileID, actionFile.BundleID, leaseStartedAt, uids, totalRowCount, duplicateUIDCount, invalidUIDCount, now); err != nil {
		return err
	}
	return flow.actionRepository.Complete(ctx, actionFileID, leaseStartedAt, now)
}

// parseBundleActionXLSX extracts sorted, unique valid UIDs and row-count metrics from an XLSX file.
func parseBundleActionXLSX(filePath string) (uids []string, totalRows, duplicateUIDs, invalidUIDs int64, err error) {
	workbook, err := excelize.OpenFile(filePath, excelize.Options{
		RawCellValue:      true,
		UnzipSizeLimit:    maxBundleActionFileUncompressedBytes,
		UnzipXMLSizeLimit: maxBundleActionFileXMLInMemoryBytes,
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer workbook.Close()

	worksheetNames := workbook.GetSheetList()
	if len(worksheetNames) == 0 {
		return nil, 0, 0, 0, errors.New("no worksheet")
	}
	rowIterator, err := workbook.Rows(worksheetNames[0])
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rowIterator.Close()
	if !rowIterator.Next() {
		if err := rowIterator.Error(); err != nil {
			return nil, 0, 0, 0, err
		}
		return nil, 0, 0, 0, errors.New("no data rows")
	}
	firstRow, err := rowIterator.Columns(excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, 0, 0, 0, err
	}
	uidColumnIndex := -1
	for columnIndex, columnName := range firstRow {
		if strings.EqualFold(strings.TrimSpace(columnName), "uid") {
			if uidColumnIndex >= 0 {
				return nil, 0, 0, 0, errors.New("duplicate uid column")
			}
			uidColumnIndex = columnIndex
		}
	}
	// A uid header is optional. A first row with a uid column is treated as the
	// header; otherwise the first column is a headerless UID list and its first
	// row is processed as data.
	currentRow := firstRow
	if uidColumnIndex < 0 {
		uidColumnIndex = 0
	} else if !rowIterator.Next() {
		if err := rowIterator.Error(); err != nil {
			return nil, 0, 0, 0, err
		}
		return nil, 0, 0, 0, errors.New("no data rows")
	} else if currentRow, err = rowIterator.Columns(excelize.Options{RawCellValue: true}); err != nil {
		return nil, 0, 0, 0, err
	}
	uniqueUIDs := map[string]struct{}{}
	for {
		totalRows++
		if totalRows > maxBundleActionFileRows {
			return nil, totalRows, duplicateUIDs, invalidUIDs, fmt.Errorf("action file exceeds the %d row limit", maxBundleActionFileRows)
		}
		var uid string
		if uidColumnIndex < len(currentRow) {
			uid = strings.TrimSpace(currentRow[uidColumnIndex])
		}
		if uid == "" || len(uid) > 255 {
			invalidUIDs++
		} else if _, exists := uniqueUIDs[uid]; exists {
			duplicateUIDs++
		} else {
			uniqueUIDs[uid] = struct{}{}
		}
		if !rowIterator.Next() {
			break
		}
		currentRow, err = rowIterator.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, totalRows, duplicateUIDs, invalidUIDs, err
		}
	}
	if err := rowIterator.Error(); err != nil {
		return nil, totalRows, duplicateUIDs, invalidUIDs, err
	}
	if len(uniqueUIDs) == 0 {
		return nil, totalRows, duplicateUIDs, invalidUIDs, errors.New("no valid uid")
	}
	uids = make([]string, 0, len(uniqueUIDs))
	for uid := range uniqueUIDs {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	return uids, totalRows, duplicateUIDs, invalidUIDs, nil
}

var _ BundleActionFlow = (*BundleActionFlowImpl)(nil)
