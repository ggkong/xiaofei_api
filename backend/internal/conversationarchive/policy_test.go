package conversationarchive

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectStorageSnapshot(mock sqlmock.Sqlmock, records int64) {
	mock.ExpectQuery(`SELECT count\(\*\),COALESCE\(sum`).WillReturnRows(sqlmock.NewRows([]string{"records", "logical_bytes"}).AddRow(records, 123456789))
	mock.ExpectQuery(`SELECT pg_total_relation_size`).WillReturnRows(sqlmock.NewRows([]string{"storage_bytes", "blocks"}).AddRow(1234567, 100))
}

func TestStorageInfoSharesStatisticsButKeepsPolicyAndCapacityLive(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := New(db, Options{Permanent: true, MaxBytes: 4 << 20})
	expectStorageSnapshot(mock, 34000)
	first, err := s.StorageInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Changing context limits must take effect even while counts are cached.
	s.options.MaxBytes = 8 << 20
	// A cached low-space flag must never pause capture after space is freed.
	s.storageValue.FreeBytes, s.storageValue.CapturePaused = 1, true
	second, err := s.StorageInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Records != first.Records || second.LogicalBytes != first.LogicalBytes || second.StorageBytes != first.StorageBytes || second.BlockCount != first.BlockCount {
		t.Fatal("cached statistics changed")
	}
	if second.MaxContextBytes != 8<<20 || second.FreeBytes == 1 || second.CapturePaused != (second.FreeBytes > 0 && second.FreeBytes < 512<<20) {
		t.Fatal("policy or capture capacity was served from the statistics cache")
	}
	// A retention change invalidates counts computed with the old policy.
	s.options.Permanent, s.options.RetentionDays = false, 7
	expectStorageSnapshot(mock, 12)
	third, err := s.StorageInfo(context.Background())
	if err != nil || third.Records != 12 || third.RetentionDays != 7 {
		t.Fatalf("retention change: %+v, %v", third, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageInfoRetriesFailedAndExpiredSnapshots(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := New(db, Options{Permanent: true})
	mock.ExpectQuery(`SELECT count\(\*\),COALESCE\(sum`).WillReturnError(context.DeadlineExceeded)
	if _, err := s.StorageInfo(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost error: %v", err)
	}
	expectStorageSnapshot(mock, 10)
	if _, err := s.StorageInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.storageLoadedAt = time.Now().Add(-time.Minute)
	expectStorageSnapshot(mock, 11)
	info, err := s.StorageInfo(context.Background())
	if err != nil || info.Records != 11 {
		t.Fatalf("stale snapshot: %+v, %v", info, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.StorageInfo(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageInfoConcurrentReadersShareOneScan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := New(db, Options{Permanent: true})
	expectStorageSnapshot(mock, 34000)
	var readers sync.WaitGroup
	for range 12 {
		readers.Go(func() {
			info, err := s.StorageInfo(context.Background())
			if err != nil || info.Records != 34000 {
				t.Errorf("concurrent statistics: %+v, %v", info, err)
			}
		})
	}
	readers.Wait()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
