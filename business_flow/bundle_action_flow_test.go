package businessflow

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/xuri/excelize/v2"
)

type bundleActionLeaseFlowRepository struct {
	repository.BundleActionRepository
	file          *models.BundleActionFile
	completed     bool
	completedWith time.Time
}

type bundleActionUploadRepository struct {
	repository.BundleActionRepository
	created  *models.BundleActionFile
	promoted bool
}

func (r *bundleActionUploadRepository) Create(_ context.Context, file *models.BundleActionFile) error {
	file.ID = 73
	r.created = file
	return nil
}

func (r *bundleActionUploadRepository) PromotePending(_ context.Context, id int64, storagePath string, _ time.Time) error {
	if r.created == nil || id != r.created.ID || r.created.Status != models.BundleActionFileStaging {
		return errors.New("action file was not staged before promotion")
	}
	if _, err := os.Stat(storagePath); err != nil {
		return err
	}
	r.promoted = true
	return nil
}

type bundleActionUploadBundleRepository struct{ repository.BundleRepository }

func (r *bundleActionUploadBundleRepository) ByID(context.Context, uint) (*models.Bundle, error) {
	return &models.Bundle{CustomerID: 17}, nil
}

func TestUploadStagesStorageBeforeMakingActionFileClaimable(t *testing.T) {
	t.Parallel()

	repo := &bundleActionUploadRepository{}
	storageRoot := t.TempDir()
	flow := &BundleActionFlowImpl{
		actionRepository: repo,
		bundleRepository: &bundleActionUploadBundleRepository{},
		storageRoot:      storageRoot,
	}

	file, err := flow.Upload(t.Context(), 17, 29, "actions.xlsx", "test", bytes.NewReader([]byte("xlsx bytes")))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if repo.created == nil || repo.created.Status != models.BundleActionFilePending {
		t.Fatalf("created file status = %v, want pending after promotion", repo.created.Status)
	}
	if !repo.promoted {
		t.Fatal("Upload did not promote the staged row")
	}
	if got, err := os.ReadFile(file.StoragePath); err != nil || string(got) != "xlsx bytes" {
		t.Fatalf("published storage = %q, %v; want staged bytes", got, err)
	}
}

func (r *bundleActionLeaseFlowRepository) ByID(context.Context, int64) (*models.BundleActionFile, error) {
	return r.file, nil
}

func (r *bundleActionLeaseFlowRepository) Complete(_ context.Context, _ int64, leaseStartedAt, _ time.Time) error {
	r.completed = true
	r.completedWith = leaseStartedAt
	return nil
}

func TestExecuteBundleActionFileFencesDeleteCompletionWithClaimedLease(t *testing.T) {
	lease := time.Date(2026, time.December, 31, 9, 12, 0, 0, time.UTC)
	repo := &bundleActionLeaseFlowRepository{file: &models.BundleActionFile{
		ID:        41,
		Status:    models.BundleActionFileDeletePending,
		StartedAt: &lease,
	}}
	flow := &BundleActionFlowImpl{actionRepository: repo}

	if err := flow.ExecuteBundleActionFile(t.Context(), 41, lease); err != nil {
		t.Fatalf("ExecuteBundleActionFile() error = %v", err)
	}
	if !repo.completed {
		t.Fatal("Complete was not called")
	}
	if !repo.completedWith.Equal(lease) {
		t.Fatalf("Complete lease = %s, want %s", repo.completedWith, lease)
	}
}

func TestExecuteBundleActionFileRejectsReclaimedLeaseBeforePublishing(t *testing.T) {
	oldLease := time.Date(2026, time.December, 31, 9, 12, 0, 0, time.UTC)
	newLease := oldLease.Add(35 * time.Minute)
	repo := &bundleActionLeaseFlowRepository{file: &models.BundleActionFile{
		ID:        41,
		Status:    models.BundleActionFileDeletePending,
		StartedAt: &newLease,
	}}
	flow := &BundleActionFlowImpl{actionRepository: repo}

	err := flow.ExecuteBundleActionFile(t.Context(), 41, oldLease)
	if !errors.Is(err, repository.ErrBundleActionFileLeaseLost) {
		t.Fatalf("ExecuteBundleActionFile() error = %v, want ErrBundleActionFileLeaseLost", err)
	}
	if repo.completed {
		t.Fatal("reclaimed worker called Complete")
	}
}

func TestParseBundleActionXLSXStreamsRowsAndPreservesCounts(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "actions.xlsx")
	xl := excelize.NewFile()
	sheet := xl.GetSheetName(0)
	if err := xl.SetSheetRow(sheet, "A1", &[]string{"uid"}); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]string{
		"A2": " uid-a ",
		"A3": "uid-a",
		"B4": "ignored",
		"A5": "uid-b",
	} {
		if err := xl.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := xl.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := xl.Close(); err != nil {
		t.Fatal(err)
	}

	uids, total, duplicates, invalid, err := parseBundleActionXLSX(path)
	if err != nil {
		t.Fatalf("parseBundleActionXLSX() error = %v", err)
	}
	if got, want := total, int64(4); got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
	if got, want := duplicates, int64(1); got != want {
		t.Errorf("duplicates = %d, want %d", got, want)
	}
	if got, want := invalid, int64(1); got != want {
		t.Errorf("invalid = %d, want %d", got, want)
	}
	if want := []string{"uid-a", "uid-b"}; !reflect.DeepEqual(uids, want) {
		t.Errorf("uids = %#v, want %#v", uids, want)
	}
}

func TestParseBundleActionXLSXAcceptsUIDsWithoutHeader(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "headerless-actions.xlsx")
	xl := excelize.NewFile()
	sheet := xl.GetSheetName(0)
	for cell, value := range map[string]string{
		"A1": " uid-a ",
		"A2": "uid-a",
		"A3": "uid-b",
	} {
		if err := xl.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := xl.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := xl.Close(); err != nil {
		t.Fatal(err)
	}

	uids, total, duplicates, invalid, err := parseBundleActionXLSX(path)
	if err != nil {
		t.Fatalf("parseBundleActionXLSX() error = %v", err)
	}
	if got, want := total, int64(3); got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
	if got, want := duplicates, int64(1); got != want {
		t.Errorf("duplicates = %d, want %d", got, want)
	}
	if got, want := invalid, int64(0); got != want {
		t.Errorf("invalid = %d, want %d", got, want)
	}
	if want := []string{"uid-a", "uid-b"}; !reflect.DeepEqual(uids, want) {
		t.Errorf("uids = %#v, want %#v", uids, want)
	}
}

func TestParseBundleActionXLSXAcceptsUIDHeaderWithWhitespaceAndCase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "headered-actions.xlsx")
	xl := excelize.NewFile()
	sheet := xl.GetSheetName(0)
	for cell, value := range map[string]string{
		"A1": " UID ",
		"A2": "uid-a",
		"A3": "uid-b",
	} {
		if err := xl.SetCellValue(sheet, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := xl.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := xl.Close(); err != nil {
		t.Fatal(err)
	}

	uids, total, duplicates, invalid, err := parseBundleActionXLSX(path)
	if err != nil {
		t.Fatalf("parseBundleActionXLSX() error = %v", err)
	}
	if got, want := total, int64(2); got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
	if got, want := duplicates, int64(0); got != want {
		t.Errorf("duplicates = %d, want %d", got, want)
	}
	if got, want := invalid, int64(0); got != want {
		t.Errorf("invalid = %d, want %d", got, want)
	}
	if want := []string{"uid-a", "uid-b"}; !reflect.DeepEqual(uids, want) {
		t.Errorf("uids = %#v, want %#v", uids, want)
	}
}
