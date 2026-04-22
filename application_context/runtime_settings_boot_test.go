package application_context

import (
	"testing"
	"time"

	"mahresources/models"
)

func TestBoot_DivergenceEmitsWarning(t *testing.T) {
	db := newTestDB(t)
	enc, _ := encodeSettingValue("int64", int64(4<<30))
	db.Create(&models.RuntimeSetting{
		Key: KeyMaxUploadSize, ValueJSON: string(enc), UpdatedAt: time.Now(),
	})
	log := &stubLogger{}
	rs := NewRuntimeSettings(db, log, buildSpecs(), defaults())
	if err := rs.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !log.contains(`runtime_setting "max_upload_size" override`) {
		t.Fatalf("want divergence WARN, got entries: %#v", log.entries)
	}
	if !log.contains(`boot flag`) {
		t.Fatalf("WARN should mention the boot flag value")
	}
}

func TestBoot_NoDivergenceWhenValuesMatch(t *testing.T) {
	db := newTestDB(t)
	// Persisted value equals the boot default from defaults().
	enc, _ := encodeSettingValue("int64", int64(2<<30))
	db.Create(&models.RuntimeSetting{
		Key: KeyMaxUploadSize, ValueJSON: string(enc), UpdatedAt: time.Now(),
	})
	log := &stubLogger{}
	rs := NewRuntimeSettings(db, log, buildSpecs(), defaults())
	_ = rs.Load()
	if log.contains(`override`) {
		t.Fatalf("no WARN expected when override equals default; got %#v", log.entries)
	}
}

func TestBoot_OutOfBoundsDroppedFromCache(t *testing.T) {
	db := newTestDB(t)
	enc, _ := encodeSettingValue("int64", int64(-1)) // below bounds
	db.Create(&models.RuntimeSetting{
		Key: KeyMaxImportSize, ValueJSON: string(enc), UpdatedAt: time.Now(),
	})
	log := &stubLogger{}
	rs := NewRuntimeSettings(db, log, buildSpecs(), defaults())
	_ = rs.Load()
	if rs.MaxImportSize() != int64(10<<30) {
		t.Fatalf("want fallback to default, got %v", rs.MaxImportSize())
	}
	if !log.contains("fails bounds") {
		t.Fatalf("want bounds-fail ERROR, got %#v", log.entries)
	}
}
